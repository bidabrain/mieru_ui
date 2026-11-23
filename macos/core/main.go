// mieru-core is the privileged half of the Mieru macOS client. Installed as a launchd
// daemon it runs as root (mihomo needs root to create the utun device and routes) and
// serves a small JSON API on a unix socket owned by the user who installed it. The same
// binary also answers the stateless calls (parse links, preview config, …) directly via
// `mieru-core call <method>`, so the app works before the daemon is installed.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bidabrain/mieru_ui/android/core/libcore"
)

var version = "dev"

const (
	defaultSocket = "/var/run/mieru-core.sock"
	defaultData   = "/Library/Application Support/Mieru"
)

const usage = `mieru-core %s — core service of the Mieru macOS client

usage:
  mieru-core daemon [-socket PATH] [-data DIR] [-owner UID] [-seed DIR]
  mieru-core call <method>     read the JSON request on stdin, print the JSON reply
  mieru-core version
`

func main() {
	log.SetFlags(log.LstdFlags)
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, usage, version)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "daemon":
		if err := cmdDaemon(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	case "call":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, usage, version)
			os.Exit(2)
		}
		body, _ := io.ReadAll(os.Stdin)
		res, err := dispatch(os.Args[2], body, false)
		_ = json.NewEncoder(os.Stdout).Encode(envelope(res, err))
	case "version", "-v", "--version":
		fmt.Println(version)
	default:
		fmt.Fprintf(os.Stderr, usage, version)
		os.Exit(2)
	}
}

type reply struct {
	OK     bool   `json:"ok"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

func envelope(res any, err error) reply {
	if err != nil {
		return reply{OK: false, Error: err.Error()}
	}
	return reply{OK: true, Result: res}
}

// request is the union of all method arguments.
type request struct {
	Text      string          `json:"text"`
	Profile   json.RawMessage `json:"profile"`
	Rule      json.RawMessage `json:"rule"`
	Options   json.RawMessage `json:"options"`
	URL       string          `json:"url"`
	TimeoutMs int32           `json:"timeoutMs"`
	After     int64           `json:"after"`
}

type status struct {
	Running     bool   `json:"running"`
	Version     string `json:"version"`
	CoreVersion string `json:"coreVersion"`
	Daemon      bool   `json:"daemon"`
	LastError   string `json:"lastError,omitempty"`
}

var (
	lastErrMu sync.Mutex
	lastErr   string
)

func setLastErr(err error) {
	lastErrMu.Lock()
	defer lastErrMu.Unlock()
	if err == nil {
		lastErr = ""
	} else {
		lastErr = err.Error()
	}
}

func raw(s string) json.RawMessage { return json.RawMessage(s) }

// dispatch runs one API method. Methods that touch the running proxy need the daemon.
func dispatch(method string, body []byte, daemon bool) (any, error) {
	var q request
	if len(body) > 0 {
		if err := json.Unmarshal(body, &q); err != nil {
			return nil, fmt.Errorf("请求格式错误: %w", err)
		}
	}
	needDaemon := func() error {
		if !daemon {
			return errors.New("核心服务未运行")
		}
		return nil
	}
	switch method {
	case "status":
		lastErrMu.Lock()
		le := lastErr
		lastErrMu.Unlock()
		return status{Running: daemon && libcore.IsRunning(), Version: version, CoreVersion: libcore.Version(), Daemon: daemon, LastError: le}, nil
	case "parse":
		s, err := libcore.ParseLinks(q.Text)
		return raw(s), err
	case "link":
		return libcore.ProfileLink(string(q.Profile))
	case "checkProfile":
		s, err := libcore.CheckProfile(string(q.Profile))
		return raw(s), err
	case "checkRule":
		s, err := libcore.CheckRule(string(q.Rule))
		return raw(s), err
	case "defaultRouting":
		return raw(libcore.DefaultRoutingJSON()), nil
	case "preview":
		return libcore.PreviewConfig(string(q.Options))
	case "delay":
		return libcore.TestDelay(string(q.Profile), q.URL, q.TimeoutMs)
	case "start":
		if err := needDaemon(); err != nil {
			return nil, err
		}
		err := libcore.StartDesktop(string(q.Options))
		setLastErr(err)
		if err == nil && libcore.IsRunning() {
			hijackSystemDNS()
		} else {
			restoreSystemDNS()
		}
		return libcore.IsRunning(), err
	case "stop":
		if err := needDaemon(); err != nil {
			return nil, err
		}
		libcore.Stop()
		restoreSystemDNS()
		return false, nil
	case "traffic":
		return raw(libcore.Traffic()), nil
	case "logs":
		return raw(libcore.Logs(q.After)), nil
	case "clearLogs":
		libcore.ClearLogs()
		return true, nil
	case "updateGeo":
		if err := needDaemon(); err != nil {
			return nil, err
		}
		if len(q.Options) > 0 {
			return true, libcore.UpdateGeoFor(string(q.Options))
		}
		return true, libcore.UpdateGeo()
	}
	return nil, fmt.Errorf("未知方法 %q", method)
}

func cmdDaemon(args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	socket := fs.String("socket", defaultSocket, "unix socket path")
	data := fs.String("data", defaultData, "data directory")
	owner := fs.Int("owner", -1, "uid that may use the socket (default: root only)")
	seed := fs.String("seed", "", "directory with bundled GeoIP.dat / GeoSite.dat")
	_ = fs.Parse(args)

	if err := os.MkdirAll(*data, 0o755); err != nil {
		return err
	}
	if *seed != "" {
		seedGeo(*seed, *data)
	}
	if err := libcore.Init(*data); err != nil {
		return err
	}
	initDNSBackup(*data)
	restoreSystemDNS() // left over from a crash

	_ = os.Remove(*socket)
	ln, err := net.Listen("unix", *socket)
	if err != nil {
		return err
	}
	if err := os.Chmod(*socket, 0o600); err != nil {
		return err
	}
	if *owner >= 0 {
		if err := os.Chown(*socket, *owner, -1); err != nil {
			return err
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		method := strings.TrimPrefix(r.URL.Path, "/")
		res, err := dispatch(method, body, true)
		if err != nil && method != "logs" && method != "traffic" && method != "status" {
			log.Printf("%s: %v", method, err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(envelope(res, err))
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sig
		log.Print("shutting down")
		libcore.Stop() // removes the utun routes
		restoreSystemDNS()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()
	log.Printf("mieru-core %s listening on %s (%s)", version, *socket, libcore.Version())
	err = srv.Serve(ln)
	_ = os.Remove(*socket)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// seedGeo copies the bundled geo databases once, so routing works offline on first run.
func seedGeo(from, to string) {
	for _, f := range []string{"GeoSite.dat", "GeoIP.dat"} {
		dst := filepath.Join(to, f)
		if st, err := os.Stat(dst); err == nil && st.Size() > 0 {
			continue
		}
		b, err := os.ReadFile(filepath.Join(from, f))
		if err != nil {
			continue
		}
		tmp := dst + ".tmp"
		if err := os.WriteFile(tmp, b, 0o644); err == nil {
			_ = os.Rename(tmp, dst)
		}
	}
}
