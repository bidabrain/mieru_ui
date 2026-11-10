// Package database implements a tiny JSON-file backed store for the panel.
package database

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// PortBinding mirrors mieru's PortBinding message.
type PortBinding struct {
	Port      int    `json:"port,omitempty"`
	PortRange string `json:"portRange,omitempty"`
	Protocol  string `json:"protocol"` // TCP | UDP
}

// Quota mirrors mieru's Quota message.
type Quota struct {
	Days      int `json:"days"`
	Megabytes int `json:"megabytes"`
}

// User is a panel-managed mieru user.
type User struct {
	ID              int64   `json:"id"`
	Name            string  `json:"name"`
	Password        string  `json:"password"`
	Enabled         bool    `json:"enabled"`
	Remark          string  `json:"remark"`
	AllowPrivateIP  bool    `json:"allowPrivateIP"`
	AllowLoopbackIP bool    `json:"allowLoopbackIP"`
	Quotas          []Quota `json:"quotas"`
	ExpireAt        int64   `json:"expireAt"` // unix milliseconds, 0 = never
	CreatedAt       int64   `json:"createdAt"`
}

// ServerConf is the panel's view of the mita server configuration.
type ServerConf struct {
	PortBindings   []PortBinding   `json:"portBindings"`
	MTU            int             `json:"mtu"`
	LoggingLevel   string          `json:"loggingLevel"`
	DualStack      string          `json:"dualStack"`
	Egress         json.RawMessage `json:"egress,omitempty"`
	TrafficPattern json.RawMessage `json:"trafficPattern,omitempty"`

	// Client side defaults, used only when generating share links / client configs.
	Multiplexing  string `json:"multiplexing"`
	HandshakeMode string `json:"handshakeMode"`
}

// Settings are panel settings.
type Settings struct {
	ListenIP      string `json:"listenIP"`
	Port          int    `json:"port"`
	BasePath      string `json:"basePath"`
	Username      string `json:"username"`
	PasswordHash  string `json:"passwordHash"`
	PasswordSalt  string `json:"passwordSalt"`
	SessionSecret string `json:"sessionSecret"`
	SessionHours  int    `json:"sessionHours"`
	PublicHost    string `json:"publicHost"`
	MitaBin       string `json:"mitaBin"`
	CertFile      string `json:"certFile"`
	KeyFile       string `json:"keyFile"`
}

// Data is the whole persisted document.
type Data struct {
	Settings       Settings   `json:"settings"`
	Server         ServerConf `json:"server"`
	Mihomo         MihomoConf `json:"mihomo"`
	Users          []User     `json:"users"`
	NextUserID     int64      `json:"nextUserID"`
	AppliedUsers   []string   `json:"appliedUsers"`
	LastApplyAt    int64      `json:"lastApplyAt"`
	LastApplyError string     `json:"lastApplyError"`
}

// Store guards the document and persists it atomically.
type Store struct {
	mu   sync.RWMutex
	path string
	data Data

	// InitialPassword is set only when Open created a brand-new database.
	InitialPassword string
}

// RandomHex returns n random bytes hex-encoded.
func RandomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func defaults() Data {
	return Data{
		Settings: Settings{
			Port:          2053,
			BasePath:      "/",
			Username:      "admin",
			SessionSecret: RandomHex(32),
			SessionHours:  24 * 7,
			MitaBin:       "mita",
		},
		Server: ServerConf{
			PortBindings: []PortBinding{{Port: 2012, Protocol: "TCP"}},
			MTU:          1400,
			LoggingLevel: "INFO",
			DualStack:    "USE_FIRST_IP",
			Multiplexing: "MULTIPLEXING_LOW",
		},
		Mihomo:     DefaultMihomo(),
		Users:      []User{},
		NextUserID: 1,
	}
}

var initialCredsPath string

// InitialCredentialsPath is where a freshly generated admin password is written.
func InitialCredentialsPath() string { return initialCredsPath }

// RemoveInitialCredentials deletes the initial credentials file once the password was changed.
func RemoveInitialCredentials() {
	if initialCredsPath != "" {
		_ = os.Remove(initialCredsPath)
	}
}

// Open loads (or creates) the store at path.
func Open(path string) (*Store, error) {
	s := &Store{path: path, data: defaults()}
	initialCredsPath = filepath.Join(filepath.Dir(path), "initial-credentials.txt")
	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
		// Never ship a well-known default password: generate one and store it in a root-only file.
		pass := RandomHex(8)
		if err := SetPassword(&s.data.Settings, pass); err != nil {
			return nil, err
		}
		if err := s.save(); err != nil {
			return nil, err
		}
		s.InitialPassword = pass
		content := fmt.Sprintf("mieru-ui initial credentials (delete this file after logging in)\nusername: %s\npassword: %s\n",
			s.data.Settings.Username, pass)
		if err := os.WriteFile(initialCredsPath, []byte(content), 0o600); err != nil {
			return nil, err
		}
		return s, nil
	case err != nil:
		return nil, err
	}
	if err := json.Unmarshal(raw, &s.data); err != nil {
		return nil, err
	}
	fillMissing(&s.data)
	return s, nil
}

func fillMissing(d *Data) {
	def := defaults()
	if d.Settings.Port == 0 {
		d.Settings.Port = def.Settings.Port
	}
	if d.Settings.BasePath == "" {
		d.Settings.BasePath = "/"
	}
	if d.Settings.SessionSecret == "" {
		d.Settings.SessionSecret = def.Settings.SessionSecret
	}
	if d.Settings.SessionHours <= 0 {
		d.Settings.SessionHours = def.Settings.SessionHours
	}
	if d.Settings.MitaBin == "" {
		d.Settings.MitaBin = "mita"
	}
	if d.Server.MTU == 0 {
		d.Server.MTU = def.Server.MTU
	}
	if d.Server.LoggingLevel == "" {
		d.Server.LoggingLevel = def.Server.LoggingLevel
	}
	if d.Server.Multiplexing == "" {
		d.Server.Multiplexing = def.Server.Multiplexing
	}
	if d.Users == nil {
		d.Users = []User{}
	}
	if d.Mihomo.MixedPort == 0 { // databases created before the mihomo template existed
		d.Mihomo = DefaultMihomo()
	}
	if d.NextUserID == 0 {
		d.NextUserID = 1
		for _, u := range d.Users {
			if u.ID >= d.NextUserID {
				d.NextUserID = u.ID + 1
			}
		}
	}
}

func (s *Store) save() error {
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func clone(d Data) Data {
	raw, _ := json.Marshal(d)
	var out Data
	_ = json.Unmarshal(raw, &out)
	return out
}

// Snapshot returns a deep copy of the current data.
func (s *Store) Snapshot() Data {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clone(s.data)
}

// Update runs fn on a copy and persists it if fn returns nil.
func (s *Store) Update(fn func(d *Data) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := clone(s.data)
	if err := fn(&d); err != nil {
		return err
	}
	old := s.data
	s.data = d
	if err := s.save(); err != nil {
		s.data = old
		return err
	}
	return nil
}
