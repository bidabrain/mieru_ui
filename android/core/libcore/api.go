// Package libcore is the gomobile-bound core of the Mieru Android client.
// Every exported function only uses gomobile-compatible types.
package libcore

import (
	"encoding/json"
	"fmt"
	"runtime/debug"
	"time"

	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/hub/executor"
)

// Init must be called once with a private writable directory.
func Init(dir string) error { return setup(dir) }

// Version returns the bundled mihomo and mieru versions.
func Version() string {
	mh, mr := C.Version, "?"
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, d := range bi.Deps {
			switch d.Path {
			case "github.com/metacubex/mihomo":
				mh = d.Version
			case "github.com/enfein/mieru/v3":
				mr = d.Version
			}
		}
	}
	return "mihomo " + mh + " · mieru " + mr
}

// ParseLinks parses mierus:// links (one per line) or a mieru client JSON config,
// returning a JSON array of profiles.
func ParseLinks(text string) (string, error) {
	ps, err := parseImport(text)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(ps)
	return string(b), err
}

// ProfileLink renders a profile (JSON) as a mierus:// share link.
func ProfileLink(profileJSON string) (string, error) {
	var p Profile
	if err := json.Unmarshal([]byte(profileJSON), &p); err != nil {
		return "", err
	}
	if err := validateProfile(&p); err != nil {
		return "", err
	}
	return profileLink(p), nil
}

// CheckProfile validates a profile (JSON) and returns its normalised form.
func CheckProfile(profileJSON string) (string, error) {
	var p Profile
	if err := json.Unmarshal([]byte(profileJSON), &p); err != nil {
		return "", err
	}
	if err := validateProfile(&p); err != nil {
		return "", err
	}
	b, err := json.Marshal(p)
	return string(b), err
}

// CheckRule validates a single routing rule (JSON) and returns its normalised form.
func CheckRule(ruleJSON string) (string, error) {
	var r Rule
	if err := json.Unmarshal([]byte(ruleJSON), &r); err != nil {
		return "", err
	}
	n, err := normalizeRule(r)
	if err != nil {
		return "", err
	}
	n.Enabled = r.Enabled
	b, err := json.Marshal(n)
	return string(b), err
}

// DefaultRoutingJSON returns the default routing settings.
func DefaultRoutingJSON() string {
	b, _ := json.Marshal(DefaultRouting())
	return string(b)
}

// PreviewConfig builds the runtime config without starting anything.
func PreviewConfig(optionsJSON string) (string, error) {
	var o Options
	if err := json.Unmarshal([]byte(optionsJSON), &o); err != nil {
		return "", err
	}
	b, err := buildConfig(o)
	return string(b), err
}

// Start launches the proxy on the given tun file descriptor (owned by the core afterwards).
func Start(optionsJSON string, tunFd int32) error {
	var o Options
	if err := json.Unmarshal([]byte(optionsJSON), &o); err != nil {
		return err
	}
	if tunFd <= 0 {
		return fmt.Errorf("invalid tun fd %d", tunFd)
	}
	fd, err := dupFd(int(tunFd))
	if err != nil {
		return fmt.Errorf("dup tun fd: %w", err)
	}
	o.TunFd = fd
	if err := start(o); err != nil {
		closeFd(fd)
		return err
	}
	return nil
}

// Stop shuts down the proxy and closes the tun.
func Stop() { stop() }

// IsRunning reports whether the proxy is active.
func IsRunning() bool { return isRunning() }

// Traffic returns {"up","down","upTotal","downTotal","conns"} as JSON.
func Traffic() string { return traffic() }

// TestDelay returns the delay in ms of a profile (JSON) against testURL.
func TestDelay(profileJSON, testURL string, timeoutMs int32) (int32, error) {
	var p Profile
	if err := json.Unmarshal([]byte(profileJSON), &p); err != nil {
		return 0, err
	}
	if timeoutMs <= 0 {
		timeoutMs = 5000
	}
	d, err := urlTest(p, testURL, time.Duration(timeoutMs)*time.Millisecond)
	return int32(d), err
}

// Logs returns {"next":seq,"lines":[...]} with lines newer than after.
func Logs(after int64) string { return logsAfter(after) }

// ClearLogs drops buffered log lines.
func ClearLogs() { clearLogs() }

// UpdateGeo downloads fresh geoip/geosite databases.
func UpdateGeo() error { return updateGeo() }

// StartRouter launches the core in OpenWrt transparent-proxy mode (options.router must be
// set). Used by the mieru-wrt router binary; the process exits to stop it.
func StartRouter(optionsJSON string) error {
	var o Options
	if err := json.Unmarshal([]byte(optionsJSON), &o); err != nil {
		return err
	}
	if o.Router == nil {
		return fmt.Errorf("options.router is required")
	}
	return start(o)
}

// UpdateGeoFor downloads fresh GeoIP/GeoSite data using the geox URLs of the given
// options (parsing the config is what registers those URLs with mihomo).
func UpdateGeoFor(optionsJSON string) error {
	var o Options
	if err := json.Unmarshal([]byte(optionsJSON), &o); err != nil {
		return err
	}
	b, err := buildConfig(o)
	if err != nil {
		return err
	}
	if _, err := executor.ParseWithBytes(b); err != nil {
		return err
	}
	return updateGeo()
}
