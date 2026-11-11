package libcore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/component/updater"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/hub/executor"
	"github.com/metacubex/mihomo/listener"
	"github.com/metacubex/mihomo/log"
	"github.com/metacubex/mihomo/tunnel"
	"github.com/metacubex/mihomo/tunnel/statistic"
)

var (
	mu      sync.Mutex
	running bool
	homeDir string
	initOne sync.Once
)

func setup(dir string) error {
	if dir == "" {
		return errors.New("home dir is empty")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	homeDir = dir
	C.SetHomeDir(dir)
	C.SetConfig(filepath.Join(dir, "config.yaml"))
	initOne.Do(func() {
		debug.SetMemoryLimit(160 << 20)
		go collectLogs()
	})
	return nil
}

func start(o Options) error {
	mu.Lock()
	defer mu.Unlock()
	if homeDir == "" {
		return errors.New("core not initialised")
	}
	if running {
		stopLocked()
	}
	buf, err := buildConfig(o)
	if err != nil {
		return err
	}
	cfg, err := executor.ParseWithBytes(buf)
	if err != nil {
		return fmt.Errorf("配置解析失败: %w", err)
	}
	executor.ApplyConfig(cfg, true)
	if o.Router != nil {
		if listener.GetPorts().TProxyPort != o.Router.TProxyPort {
			return fmt.Errorf("TPROXY 端口 %d 监听失败（可能被占用），请查看日志", o.Router.TProxyPort)
		}
	} else if !listener.GetTunConf().Enable {
		executor.Shutdown()
		return errors.New("TUN 启动失败，请查看日志")
	}
	running = true
	log.Infoln("[mieru] started with profile %s", o.Profile.Name)
	return nil
}

func stopLocked() {
	if !running {
		return
	}
	running = false
	// Close the tun first so the system VPN goes away at once. Closing connections
	// afterwards can be slow: a graceful mieru close waits up to ~1s per session for
	// its close frame to be sent, so doing it serially before the tun could keep the
	// VPN up for minutes on a busy link.
	executor.Shutdown()

	var conns []statistic.Tracker
	statistic.DefaultManager.Range(func(c statistic.Tracker) bool {
		conns = append(conns, c)
		return true
	})
	var outbounds []C.Proxy
	for _, p := range tunnel.Proxies() {
		if p.Type() == C.Mieru {
			outbounds = append(outbounds, p)
		}
	}
	go func() {
		var wg sync.WaitGroup
		for _, c := range conns {
			wg.Add(1)
			go func(c statistic.Tracker) {
				defer wg.Done()
				_ = c.Close()
			}(c)
		}
		wg.Wait()
		// Stop the mieru clients too; otherwise each restart leaks one per port.
		for _, p := range outbounds {
			_ = p.Close()
		}
		debug.FreeOSMemory()
	}()
}

func stop() {
	mu.Lock()
	defer mu.Unlock()
	stopLocked()
}

func isRunning() bool {
	mu.Lock()
	defer mu.Unlock()
	return running
}

type trafficInfo struct {
	Up        int64 `json:"up"`
	Down      int64 `json:"down"`
	UpTotal   int64 `json:"upTotal"`
	DownTotal int64 `json:"downTotal"`
	Conns     int   `json:"conns"`
}

func traffic() string {
	up, down := statistic.DefaultManager.Now()
	ut, dt := statistic.DefaultManager.Total()
	n := 0
	statistic.DefaultManager.Range(func(statistic.Tracker) bool { n++; return true })
	b, _ := json.Marshal(trafficInfo{up, down, ut, dt, n})
	return string(b)
}

// urlTest measures the delay of the first binding of a profile (the app itself is
// excluded from the VPN, so this always dials the server directly).
func urlTest(p Profile, testURL string, timeout time.Duration) (int, error) {
	if err := validateProfile(&p); err != nil {
		return 0, err
	}
	proxies, _ := proxyMaps(p)
	best := -1
	var lastErr error
	for _, m := range proxies {
		px, err := adapter.ParseProxy(m)
		if err != nil {
			lastErr = err
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		d, err := px.URLTest(ctx, testURL, nil)
		cancel()
		_ = px.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if best < 0 || int(d) < best {
			best = int(d)
		}
	}
	if best < 0 {
		if lastErr == nil {
			lastErr = errors.New("测速失败")
		}
		return 0, lastErr
	}
	return best, nil
}

func updateGeo() error {
	return updater.UpdateGeoDatabases()
}
