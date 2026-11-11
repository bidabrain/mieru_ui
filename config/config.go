// Package config holds build info and filesystem locations.
package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// Version is overridden at build time with -ldflags "-X mieru-ui/config.Version=..."
var Version = "0.1.3"

// DataDir returns the directory that stores the panel database and generated files.
func DataDir() string {
	if d := os.Getenv("MIERU_UI_DATA"); d != "" {
		return d
	}
	if runtime.GOOS == "windows" {
		return "data"
	}
	return "/etc/mieru-ui"
}

// DBPath is the panel database (a JSON document).
func DBPath() string { return filepath.Join(DataDir(), "mieru-ui.json") }

// MitaConfigPath is where the generated mita server config is written before applying.
func MitaConfigPath() string { return filepath.Join(DataDir(), "mita-server.json") }
