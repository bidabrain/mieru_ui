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
	if !listener.GetTunConf().Enable {
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
	statistic.DefaultManager.Range(func(c statistic.Tracker) bool {
		_ = c.Close()
		return true
	})
	executor.Shutdown()
	running = false
	debug.FreeOSMemory()
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
