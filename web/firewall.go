package web

import (
	"net/http"
	"strconv"
	"strings"

	"mieru-ui/service"
)

// panelPorts are ports that must stay open for the panel itself.
func (s *Server) panelPorts() []service.RequiredPort {
	d := s.store.Snapshot()
	return []service.RequiredPort{{Label: "面板", Port: strconv.Itoa(d.Settings.Port), Proto: "tcp"}}
}

// mieruPorts are the ports mita listens on.
func (s *Server) mieruPorts() []service.RequiredPort {
	d := s.store.Snapshot()
	var out []service.RequiredPort
	for _, b := range d.Server.PortBindings {
		port := strconv.Itoa(b.Port)
		if b.PortRange != "" {
			port = b.PortRange
		}
		out = append(out, service.RequiredPort{Label: "mieru", Port: port, Proto: strings.ToLower(b.Protocol)})
	}
	return out
}

func (s *Server) firewallStatus(w http.ResponseWriter, r *http.Request) {
	ok(w, service.GetFirewallStatus(append(s.panelPorts(), s.mieruPorts()...)))
}

func (s *Server) firewallAllow(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Port  string `json:"port"`
		Proto string `json:"proto"`
	}
	if err := readJSON(r, &req); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if err := service.FirewallAllow(req.Port, req.Proto); err != nil {
		fail(w, 400, err.Error())
		return
	}
	okMsg(w, "已放行 "+req.Port+"/"+req.Proto, nil)
}

func (s *Server) firewallOpenMieru(w http.ResponseWriter, r *http.Request) {
	ports := append(s.panelPorts(), s.mieruPorts()...)
	for _, p := range ports {
		if err := service.FirewallAllow(p.Port, p.Proto); err != nil {
			fail(w, 400, err.Error())
			return
		}
	}
	okMsg(w, "已放行面板端口和全部 mieru 端口", nil)
}

func (s *Server) firewallDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID    string `json:"id"`
		Force bool   `json:"force"`
	}
	if err := readJSON(r, &req); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if !req.Force {
		st := service.GetFirewallStatus(nil)
		critical := s.panelPorts()
		for _, p := range st.SSHPorts {
			critical = append(critical, service.RequiredPort{Label: "SSH", Port: strconv.Itoa(p), Proto: "tcp"})
		}
		if what := service.RuleProtectsCritical(st, req.ID, critical); what != "" {
			writeJSON(w, 200, apiResp{Success: false, Msg: "删除该规则会关闭 " + what + "，可能导致无法连接服务器", Obj: "need_force"})
			return
		}
	}
	if err := service.FirewallDelete(req.ID); err != nil {
		fail(w, 400, err.Error())
		return
	}
	okMsg(w, "规则已删除", nil)
}

func (s *Server) firewallEnable(w http.ResponseWriter, r *http.Request) {
	if err := service.FirewallEnable(s.panelPorts()); err != nil {
		fail(w, 400, err.Error())
		return
	}
	okMsg(w, "防火墙已启用（已自动放行 SSH 和面板端口）", nil)
}

func (s *Server) firewallDisable(w http.ResponseWriter, r *http.Request) {
	if err := service.FirewallDisable(); err != nil {
		fail(w, 400, err.Error())
		return
	}
	okMsg(w, "防火墙已关闭", nil)
}

func (s *Server) firewallInstall(w http.ResponseWriter, r *http.Request) {
	out, err := service.InstallUFW()
	if err != nil {
		writeJSON(w, 200, apiResp{Success: false, Msg: err.Error(), Obj: out})
		return
	}
	okMsg(w, "ufw 已安装（尚未启用）", out)
}
