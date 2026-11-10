package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"mieru-ui/config"
	"mieru-ui/database"
	"mieru-ui/service"
)

func (s *Server) mita() service.Mita {
	return service.Mita{Bin: s.store.Snapshot().Settings.MitaBin}
}

// applyAndRespond pushes config to mita after a data change. The change itself is
// already saved, so an apply failure is reported as a warning rather than an error.
func (s *Server) applyAndRespond(w http.ResponseWriter, okText string, obj any) {
	if _, err := service.Apply(s.store); err != nil {
		writeJSON(w, 200, apiResp{Success: true, Msg: okText + "，但应用到 mita 失败: " + err.Error(), Obj: obj})
		return
	}
	okMsg(w, okText+"，已应用到 mita", obj)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	d := s.store.Snapshot()
	ok(w, map[string]any{
		"username":        d.Settings.Username,
		"version":         config.Version,
		"defaultPassword": database.CheckLogin(d.Settings, "admin", "admin"),
	})
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	ok(w, service.GetSystemStatus())
}

// ---------- mita ----------

func (s *Server) mitaInfo(w http.ResponseWriter, r *http.Request) {
	d := s.store.Snapshot()
	m := service.Mita{Bin: d.Settings.MitaBin}
	info := map[string]any{
		"installed":      m.Installed(),
		"lastApplyAt":    d.LastApplyAt,
		"lastApplyError": d.LastApplyError,
		"activeUsers":    len(service.ActiveUsers(&d)),
		"totalUsers":     len(d.Users),
	}
	if m.Installed() {
		v, _ := m.Version()
		st, running, err := m.Status()
		if err != nil {
			st = err.Error()
		}
		info["version"] = v
		info["statusText"] = st
		info["running"] = running
	}
	ok(w, info)
}

func (s *Server) mitaAction(w http.ResponseWriter, r *http.Request) {
	m := s.mita()
	var steps []string
	var err error
	switch r.PathValue("action") {
	case "apply":
		var res service.ApplyResult
		res, err = service.Apply(s.store)
		steps = res.Steps
	case "start":
		var res service.ApplyResult
		res, err = service.Apply(s.store)
		steps = res.Steps
		if err == nil {
			var out string
			out, err = m.Start()
			steps = append(steps, "start: "+out)
		}
	case "stop":
		var out string
		out, err = m.Stop()
		steps = append(steps, "stop: "+out)
	case "restart":
		out, _ := m.Stop()
		steps = append(steps, "stop: "+out)
		var res service.ApplyResult
		res, err = service.Apply(s.store)
		steps = append(steps, res.Steps...)
		if err == nil {
			out, err = m.Start()
			steps = append(steps, "start: "+out)
		}
	default:
		fail(w, 404, "未知操作")
		return
	}
	if err != nil {
		writeJSON(w, 200, apiResp{Success: false, Msg: err.Error(), Obj: steps})
		return
	}
	okMsg(w, "操作成功", steps)
}

func (s *Server) mitaUsers(w http.ResponseWriter, r *http.Request) {
	out, err := s.mita().GetUsers()
	if err != nil && out == "" {
		out = err.Error()
	}
	ok(w, out)
}

func (s *Server) mitaMetrics(w http.ResponseWriter, r *http.Request) {
	out, err := s.mita().GetMetrics()
	if err != nil && out == "" {
		out = err.Error()
	}
	ok(w, out)
}

// ---------- users ----------

var userNameRe = regexp.MustCompile(`^[A-Za-z0-9._@\-]{1,64}$`)

func validateUser(u *database.User, all []database.User) error {
	u.Name = strings.TrimSpace(u.Name)
	u.Remark = strings.TrimSpace(u.Remark)
	if u.Name == "" {
		u.Name = "u" + database.RandomHex(4)
	}
	if !userNameRe.MatchString(u.Name) {
		return errors.New("用户名只能包含字母、数字以及 . _ @ -，长度 1-64")
	}
	if u.Password == "" {
		u.Password = database.RandomHex(12)
	}
	if len(u.Password) > 128 || strings.ContainsAny(u.Password, "\r\n") {
		return errors.New("密码过长或包含非法字符")
	}
	for _, q := range u.Quotas {
		if q.Days <= 0 || q.Megabytes <= 0 {
			return errors.New("流量配额的天数和流量必须大于 0")
		}
	}
	if u.ExpireAt < 0 {
		u.ExpireAt = 0
	}
	for _, o := range all {
		if o.ID != u.ID && o.Name == u.Name {
			return fmt.Errorf("用户名 %s 已存在", u.Name)
		}
	}
	return nil
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	ok(w, s.store.Snapshot().Users)
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var u database.User
	if err := readJSON(r, &u); err != nil {
		fail(w, 400, err.Error())
		return
	}
	err := s.store.Update(func(d *database.Data) error {
		u.ID = d.NextUserID
		if err := validateUser(&u, d.Users); err != nil {
			return err
		}
		d.NextUserID++
		u.CreatedAt = time.Now().UnixMilli()
		d.Users = append(d.Users, u)
		return nil
	})
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	s.applyAndRespond(w, "用户已添加", u)
}

func (s *Server) batchCreateUsers(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Count    int              `json:"count"`
		Prefix   string           `json:"prefix"`
		Quotas   []database.Quota `json:"quotas"`
		ExpireAt int64            `json:"expireAt"`
		Remark   string           `json:"remark"`
	}
	if err := readJSON(r, &req); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if req.Count < 1 || req.Count > 500 {
		fail(w, 400, "数量需在 1-500 之间")
		return
	}
	if req.Prefix == "" {
		req.Prefix = "user"
	}
	err := s.store.Update(func(d *database.Data) error {
		for i := 0; i < req.Count; i++ {
			u := database.User{
				ID:       d.NextUserID,
				Name:     req.Prefix + "-" + database.RandomHex(3),
				Enabled:  true,
				Remark:   req.Remark,
				Quotas:   req.Quotas,
				ExpireAt: req.ExpireAt,
			}
			if err := validateUser(&u, d.Users); err != nil {
				return err
			}
			u.CreatedAt = time.Now().UnixMilli()
			d.NextUserID++
			d.Users = append(d.Users, u)
		}
		return nil
	})
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	s.applyAndRespond(w, fmt.Sprintf("已添加 %d 个用户", req.Count), nil)
}

func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func findUser(d *database.Data, id int64) int {
	for i, u := range d.Users {
		if u.ID == id {
			return i
		}
	}
	return -1
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, 400, "无效 ID")
		return
	}
	var u database.User
	if err := readJSON(r, &u); err != nil {
		fail(w, 400, err.Error())
		return
	}
	err = s.store.Update(func(d *database.Data) error {
		i := findUser(d, id)
		if i < 0 {
			return errors.New("用户不存在")
		}
		u.ID = id
		u.CreatedAt = d.Users[i].CreatedAt
		if err := validateUser(&u, d.Users); err != nil {
			return err
		}
		d.Users[i] = u
		return nil
	})
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	s.applyAndRespond(w, "用户已更新", u)
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, 400, "无效 ID")
		return
	}
	err = s.store.Update(func(d *database.Data) error {
		i := findUser(d, id)
		if i < 0 {
			return errors.New("用户不存在")
		}
		d.Users = append(d.Users[:i], d.Users[i+1:]...)
		return nil
	})
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	s.applyAndRespond(w, "用户已删除", nil)
}

func (s *Server) enableUser(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, 400, "无效 ID")
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := readJSON(r, &req); err != nil {
		fail(w, 400, err.Error())
		return
	}
	err = s.store.Update(func(d *database.Data) error {
		i := findUser(d, id)
		if i < 0 {
			return errors.New("用户不存在")
		}
		d.Users[i].Enabled = req.Enabled
		return nil
	})
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	if req.Enabled {
		s.applyAndRespond(w, "用户已启用", nil)
	} else {
		s.applyAndRespond(w, "用户已禁用", nil)
	}
}

func (s *Server) shareUser(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, 400, "无效 ID")
		return
	}
	d := s.store.Snapshot()
	i := findUser(&d, id)
	if i < 0 {
		fail(w, 404, "用户不存在")
		return
	}
	u := d.Users[i]
	host := s.shareHost(r, d.Settings.PublicHost)
	clientJSON, err := service.ClientJSON(u, host, d.Server)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	// The full mihomo config is rendered from the saved template every time, so it
	// always reflects the current template, ports and user credentials.
	mihomo, err := service.BuildMihomo(u, host, d.Server, d.Mihomo)
	if err != nil {
		mihomo = "# " + err.Error() + "\n"
	}
	ok(w, map[string]any{
		"host":          host,
		"link":          service.MierusLink(u, host, d.Server),
		"clientJSON":    string(clientJSON),
		"mihomo":        mihomo,
		"mihomoProxies": service.MihomoYAML(u, host, d.Server),
	})
}

func (s *Server) qrcode(w http.ResponseWriter, r *http.Request) {
	text := r.URL.Query().Get("text")
	if text == "" || len(text) > 2048 {
		fail(w, 400, "内容为空或过长")
		return
	}
	png, err := qrcode.Encode(text, qrcode.Medium, 320)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(png)
}

// ---------- server config ----------

var (
	portRangeRe   = regexp.MustCompile(`^(\d{1,5})-(\d{1,5})$`)
	loggingLevels = set("TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL")
	dualStacks    = set("", "USE_FIRST_IP", "PREFER_IPv4", "PREFER_IPv6", "ONLY_IPv4", "ONLY_IPv6")
	multiplexings = set("", "MULTIPLEXING_OFF", "MULTIPLEXING_LOW", "MULTIPLEXING_MIDDLE", "MULTIPLEXING_HIGH")
	handshakes    = set("", "HANDSHAKE_STANDARD", "HANDSHAKE_NO_WAIT")
)

func set(v ...string) map[string]bool {
	m := map[string]bool{}
	for _, s := range v {
		m[s] = true
	}
	return m
}

func validateServer(sc *database.ServerConf) error {
	if len(sc.PortBindings) == 0 {
		return errors.New("至少需要一个监听端口")
	}
	type span struct{ lo, hi int }
	used := map[string][]span{}
	for i := range sc.PortBindings {
		b := &sc.PortBindings[i]
		b.Protocol = strings.ToUpper(strings.TrimSpace(b.Protocol))
		if b.Protocol != "TCP" && b.Protocol != "UDP" {
			return fmt.Errorf("协议必须是 TCP 或 UDP")
		}
		b.PortRange = strings.TrimSpace(b.PortRange)
		var lo, hi int
		if b.PortRange != "" {
			m := portRangeRe.FindStringSubmatch(b.PortRange)
			if m == nil {
				return fmt.Errorf("端口范围格式错误: %s（示例 2000-2010）", b.PortRange)
			}
			lo, _ = strconv.Atoi(m[1])
			hi, _ = strconv.Atoi(m[2])
			if lo < 1 || hi > 65535 || lo >= hi {
				return fmt.Errorf("端口范围无效: %s", b.PortRange)
			}
			b.Port = 0
		} else {
			if b.Port < 1 || b.Port > 65535 {
				return fmt.Errorf("端口无效: %d", b.Port)
			}
			lo, hi = b.Port, b.Port
		}
		for _, o := range used[b.Protocol] {
			if lo <= o.hi && o.lo <= hi {
				return fmt.Errorf("%s 端口 %d-%d 与其他端口重叠", b.Protocol, lo, hi)
			}
		}
		used[b.Protocol] = append(used[b.Protocol], span{lo, hi})
	}
	if sc.MTU < 1280 || sc.MTU > 1400 {
		return errors.New("MTU 必须在 1280-1400 之间")
	}
	if !loggingLevels[sc.LoggingLevel] {
		return errors.New("日志级别无效")
	}
	if !dualStacks[sc.DualStack] || !multiplexings[sc.Multiplexing] || !handshakes[sc.HandshakeMode] {
		return errors.New("DNS / 多路复用 / 握手模式取值无效")
	}
	for name, raw := range map[string]*json.RawMessage{"egress": &sc.Egress, "trafficPattern": &sc.TrafficPattern} {
		t := strings.TrimSpace(string(*raw))
		if t == "" || t == "null" || t == "{}" || t == `""` {
			*raw = nil
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal(*raw, &obj); err != nil {
			return fmt.Errorf("%s 必须是 JSON 对象: %v", name, err)
		}
	}
	return nil
}

func (s *Server) getServer(w http.ResponseWriter, r *http.Request) {
	ok(w, s.store.Snapshot().Server)
}

func (s *Server) putServer(w http.ResponseWriter, r *http.Request) {
	var sc database.ServerConf
	if err := readJSON(r, &sc); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if err := validateServer(&sc); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if err := s.store.Update(func(d *database.Data) error { d.Server = sc; return nil }); err != nil {
		fail(w, 500, err.Error())
		return
	}
	s.applyAndRespond(w, "服务端配置已保存", sc)
}

func (s *Server) previewServer(w http.ResponseWriter, r *http.Request) {
	d := s.store.Snapshot()
	raw, _ := json.MarshalIndent(service.BuildServerConfig(&d), "", "  ")
	effective := ""
	if m := s.mita(); m.Installed() {
		out, err := m.Describe()
		if err != nil && out == "" {
			out = err.Error()
		}
		effective = out
	}
	ok(w, map[string]any{"generated": string(raw), "effective": effective})
}

// ---------- panel settings ----------

type publicSettings struct {
	ListenIP     string `json:"listenIP"`
	Port         int    `json:"port"`
	BasePath     string `json:"basePath"`
	SessionHours int    `json:"sessionHours"`
	PublicHost   string `json:"publicHost"`
	CertFile     string `json:"certFile"`
	KeyFile      string `json:"keyFile"`
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	st := s.store.Snapshot().Settings
	ok(w, map[string]any{
		"listenIP": st.ListenIP, "port": st.Port, "basePath": st.BasePath, "sessionHours": st.SessionHours,
		"publicHost": st.PublicHost, "certFile": st.CertFile, "keyFile": st.KeyFile,
		// Shown read-only: the executed binary can only be changed from the server's CLI.
		"mitaBin": st.MitaBin,
	})
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var p publicSettings
	if err := readJSON(r, &p); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if p.Port < 1 || p.Port > 65535 {
		fail(w, 400, "端口无效")
		return
	}
	if p.SessionHours < 1 {
		p.SessionHours = 24
	}
	if (p.CertFile == "") != (p.KeyFile == "") {
		fail(w, 400, "证书和私钥需同时填写")
		return
	}
	for _, f := range []string{p.CertFile, p.KeyFile} {
		if f != "" {
			if _, err := os.Stat(f); err != nil {
				fail(w, 400, "文件不存在: "+f)
				return
			}
		}
	}
	p.BasePath = NormalizeBasePath(p.BasePath)
	err := s.store.Update(func(d *database.Data) error {
		st := &d.Settings
		st.ListenIP, st.Port, st.BasePath = strings.TrimSpace(p.ListenIP), p.Port, p.BasePath
		st.SessionHours, st.PublicHost = p.SessionHours, strings.TrimSpace(p.PublicHost)
		st.CertFile, st.KeyFile = strings.TrimSpace(p.CertFile), strings.TrimSpace(p.KeyFile)
		return nil
	})
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	okMsg(w, "设置已保存，监听地址/端口/路径/证书需重启面板后生效", nil)
}

func (s *Server) putCredentials(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OldPassword string `json:"oldPassword"`
		Username    string `json:"username"`
		Password    string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		fail(w, 400, err.Error())
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || len(req.Password) < 8 {
		fail(w, 400, "用户名不能为空，密码至少 8 位")
		return
	}
	err := s.store.Update(func(d *database.Data) error {
		if !database.CheckLogin(d.Settings, d.Settings.Username, req.OldPassword) {
			return errors.New("原密码错误")
		}
		d.Settings.Username = req.Username
		return database.SetPassword(&d.Settings, req.Password)
	})
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	database.RemoveInitialCredentials()
	okMsg(w, "账号已修改，请重新登录", nil)
}

func (s *Server) restartPanel(w http.ResponseWriter, r *http.Request) {
	okMsg(w, "面板正在重启…", nil)
	go func() {
		time.Sleep(time.Second)
		log.Println("restart requested from web UI, exiting (systemd will restart us)")
		os.Exit(0)
	}()
}

// ---------- logs & backup ----------

func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	if n <= 0 || n > 2000 {
		n = 200
	}
	unit := "mita"
	if r.URL.Query().Get("unit") == "panel" {
		unit = "mieru-ui"
	}
	if runtime.GOOS != "linux" {
		ok(w, "日志查看仅支持 Linux (journalctl)")
		return
	}
	out, err := exec.Command("journalctl", "-u", unit, "--no-pager", "-n", strconv.Itoa(n)).CombinedOutput()
	if err != nil && len(out) == 0 {
		ok(w, err.Error())
		return
	}
	ok(w, string(out))
}

type backupDoc struct {
	App     string               `json:"app"`
	Version string               `json:"version"`
	Server  database.ServerConf  `json:"server"`
	Users   []database.User      `json:"users"`
	Mihomo  *database.MihomoConf `json:"mihomo,omitempty"`
}

func (s *Server) backup(w http.ResponseWriter, r *http.Request) {
	d := s.store.Snapshot()
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="mieru-ui-backup-%s.json"`, time.Now().Format("20060102-150405")))
	writeJSON(w, 200, backupDoc{App: "mieru-ui", Version: config.Version, Server: d.Server, Users: d.Users, Mihomo: &d.Mihomo})
}

func (s *Server) restore(w http.ResponseWriter, r *http.Request) {
	var b backupDoc
	if err := readJSON(r, &b); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if b.App != "mieru-ui" {
		fail(w, 400, "不是 mieru-ui 备份文件")
		return
	}
	if err := validateServer(&b.Server); err != nil {
		fail(w, 400, "备份中的服务端配置无效: "+err.Error())
		return
	}
	err := s.store.Update(func(d *database.Data) error {
		var users []database.User
		var next int64 = 1
		for _, u := range b.Users {
			u.ID = next // renumber to guarantee unique IDs
			if err := validateUser(&u, users); err != nil {
				return err
			}
			users = append(users, u)
			next++
		}
		if users == nil {
			users = []database.User{}
		}
		d.Server, d.Users, d.NextUserID = b.Server, users, next
		if b.Mihomo != nil {
			if err := service.ValidateMihomo(b.Mihomo); err != nil {
				return fmt.Errorf("备份中的 mihomo 模板无效: %v", err)
			}
			d.Mihomo = *b.Mihomo
		}
		return nil
	})
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	s.applyAndRespond(w, "备份已恢复", nil)
}
