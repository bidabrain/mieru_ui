package web

import (
	"net/http"
	"strings"

	"mieru-ui/database"
	"mieru-ui/service"
)

func (s *Server) getMihomo(w http.ResponseWriter, r *http.Request) {
	ok(w, map[string]any{
		"conf":     s.store.Snapshot().Mihomo,
		"defaults": database.DefaultMihomo(),
		"services": service.MihomoServices,
	})
}

func (s *Server) putMihomo(w http.ResponseWriter, r *http.Request) {
	var c database.MihomoConf
	if err := readJSON(r, &c); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if err := service.ValidateMihomo(&c); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if err := s.store.Update(func(d *database.Data) error { d.Mihomo = c; return nil }); err != nil {
		fail(w, 500, err.Error())
		return
	}
	okMsg(w, "mihomo 配置模板已保存，用户分享中的配置会按此实时生成", c)
}

// previewMihomo renders an unsaved template for a user, so the page can show a live preview.
func (s *Server) previewMihomo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Conf   database.MihomoConf `json:"conf"`
		UserID int64               `json:"userId"`
	}
	if err := readJSON(r, &req); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if err := service.ValidateMihomo(&req.Conf); err != nil {
		fail(w, 400, err.Error())
		return
	}
	d := s.store.Snapshot()
	u := database.User{Name: "example", Password: "example-password"}
	if i := findUser(&d, req.UserID); i >= 0 {
		u = d.Users[i]
	}
	out, err := service.BuildMihomo(u, s.shareHost(r, d.Settings.PublicHost), d.Server, req.Conf)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	ok(w, out)
}

// shareHost picks the address clients should connect to.
func (s *Server) shareHost(r *http.Request, publicHost string) string {
	if h := strings.TrimSpace(r.URL.Query().Get("host")); h != "" {
		return h
	}
	if publicHost != "" {
		return publicHost
	}
	return service.HostFromRequest(r.Host)
}
