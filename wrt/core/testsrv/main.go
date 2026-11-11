// Minimal mieru server used by the OpenWrt end-to-end test in CI (TCP CONNECT only).
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/enfein/mieru/v3/apis/constant"
	"github.com/enfein/mieru/v3/apis/model"
	"github.com/enfein/mieru/v3/apis/server"
	"github.com/enfein/mieru/v3/pkg/appctl/appctlpb"
	"github.com/enfein/mieru/v3/pkg/common"
	"google.golang.org/protobuf/proto"
)

func main() {
	mirror := flag.String("mirror", "", "also serve an HTTP reverse proxy to this https origin on :8080 (package feeds for the test container)")
	flag.Parse()
	if *mirror != "" {
		u, err := url.Parse(*mirror)
		if err != nil {
			log.Fatal(err)
		}
		rp := httputil.NewSingleHostReverseProxy(u)
		dir := rp.Director
		rp.Director = func(r *http.Request) { dir(r); r.Host = u.Host }
		go func() { log.Fatal(http.ListenAndServe(":8080", rp)) }()
		fmt.Println("mirror :8080 ->", *mirror)
	}
	s := server.NewServer()
	if err := s.Store(&server.ServerConfig{Config: &appctlpb.ServerConfig{
		ListenIPAddress: proto.String("0.0.0.0"),
		PortBindings:    []*appctlpb.PortBinding{{Port: proto.Int32(2012), Protocol: appctlpb.TransportProtocol_TCP.Enum()}},
		Users:           []*appctlpb.User{{Name: proto.String("alice"), Password: proto.String("testpass")}},
	}}); err != nil {
		log.Fatal(err)
	}
	if err := s.Start(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("listening 0.0.0.0:2012 TCP")
	for {
		c, req, err := s.Accept()
		if err != nil {
			log.Println("accept:", err)
			continue
		}
		go handle(c, req)
	}
}

func handle(c net.Conn, req *model.Request) {
	defer c.Close()
	if req.Command != constant.Socks5ConnectCmd {
		log.Println("unsupported cmd", req.Command)
		return
	}
	t, err := net.Dial("tcp", req.DstAddr.String())
	if err != nil {
		log.Println("dial", req.DstAddr.String(), err)
		(&model.Response{Reply: constant.Socks5ReplyHostUnreachable}).WriteToSocks5(c)
		return
	}
	defer t.Close()
	l := t.LocalAddr().(*net.TCPAddr)
	if err := (&model.Response{Reply: constant.Socks5ReplySuccess, BindAddr: model.AddrSpec{IP: l.IP, Port: l.Port}}).WriteToSocks5(c); err != nil {
		return
	}
	log.Println("CONNECT", req.DstAddr.String())
	common.BidiCopy(c, t)
}
