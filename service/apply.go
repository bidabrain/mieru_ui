package service

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"mieru-ui/config"
	"mieru-ui/database"
)

var applyMu sync.Mutex

// ActiveUsers returns the users that should exist in mita right now.
func ActiveUsers(d *database.Data) []database.User {
	now := time.Now().UnixMilli()
	var out []database.User
	for _, u := range d.Users {
		if !u.Enabled {
			continue
		}
		if u.ExpireAt > 0 && u.ExpireAt <= now {
			continue
		}
		out = append(out, u)
	}
	return out
}

// BuildServerConfig renders the mita server config JSON for the given data.
func BuildServerConfig(d *database.Data) map[string]any {
	sc := d.Server
	users := []map[string]any{}
	for _, u := range ActiveUsers(d) {
		m := map[string]any{
			"name":     u.Name,
			"password": u.Password,
		}
		if u.AllowPrivateIP {
			m["allowPrivateIP"] = true
		}
		if u.AllowLoopbackIP {
			m["allowLoopbackIP"] = true
		}
		if len(u.Quotas) > 0 {
			m["quotas"] = u.Quotas
		}
		users = append(users, m)
	}
	bindings := []map[string]any{}
	for _, b := range sc.PortBindings {
		m := map[string]any{"protocol": b.Protocol}
		if b.PortRange != "" {
			m["portRange"] = b.PortRange
		} else {
			m["port"] = b.Port
		}
		bindings = append(bindings, m)
	}
	cfg := map[string]any{
		"portBindings": bindings,
		"users":        users,
		"loggingLevel": sc.LoggingLevel,
		"mtu":          sc.MTU,
	}
	if sc.DualStack != "" {
		cfg["dns"] = map[string]any{"dualStack": sc.DualStack}
	}
	if len(sc.Egress) > 0 && string(sc.Egress) != "null" {
		cfg["egress"] = sc.Egress
	}
	if len(sc.TrafficPattern) > 0 && string(sc.TrafficPattern) != "null" {
		cfg["trafficPattern"] = sc.TrafficPattern
	}
	return cfg
}

// ApplyResult describes what happened during Apply.
type ApplyResult struct {
	Steps []string `json:"steps"`
}

// Apply pushes the panel state into mita:
//  1. write the full config and `mita apply config` it (users are merged by name, port bindings replaced)
//  2. `mita delete user` every user that the panel applied before but is no longer active
//  3. `mita reload` if the proxy is running
func Apply(store *database.Store) (ApplyResult, error) {
	applyMu.Lock()
	defer applyMu.Unlock()

	var res ApplyResult
	d := store.Snapshot()
	mita := Mita{Bin: d.Settings.MitaBin}

	err := func() error {
		if !mita.Installed() {
			return fmt.Errorf("未找到 mita 可执行文件 (%s)，请先安装 mieru 服务端", mita.bin())
		}
		cfg := BuildServerConfig(&d)
		raw, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(config.MitaConfigPath(), raw, 0o600); err != nil {
			return err
		}
		out, err := mita.ApplyConfig(config.MitaConfigPath())
		res.Steps = append(res.Steps, "apply config: "+orOK(out))
		if err != nil {
			return err
		}

		active := map[string]bool{}
		for _, u := range ActiveUsers(&d) {
			active[u.Name] = true
		}
		for _, name := range d.AppliedUsers {
			if active[name] {
				continue
			}
			out, err := mita.DeleteUser(name)
			if err != nil && !strings.Contains(strings.ToLower(out+err.Error()), "not found") {
				res.Steps = append(res.Steps, "delete user "+name+": "+err.Error())
				return err
			}
			res.Steps = append(res.Steps, "delete user "+name+": "+orOK(out))
		}

		if _, running, _ := mita.Status(); running {
			out, err := mita.Reload()
			res.Steps = append(res.Steps, "reload: "+orOK(out))
			if err != nil {
				return err
			}
		}
		return nil
	}()

	names := []string{}
	for _, u := range ActiveUsers(&d) {
		names = append(names, u.Name)
	}
	sort.Strings(names)
	_ = store.Update(func(nd *database.Data) error {
		nd.LastApplyAt = time.Now().UnixMilli()
		if err != nil {
			nd.LastApplyError = err.Error()
		} else {
			nd.LastApplyError = ""
			nd.AppliedUsers = names
		}
		return nil
	})
	return res, err
}

func orOK(s string) string {
	if s == "" {
		return "OK"
	}
	return s
}
