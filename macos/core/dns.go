package main

import (
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// tunDNS is inside mihomo's tun range (198.18.0.0/16, routed into the tun), so queries
// sent there are captured by dns-hijack. Without this, macOS keeps asking the LAN
// resolver directly (the connected-subnet route beats the tun routes) and fake-ip and
// the anti-pollution DNS never see the queries.
const tunDNS = "198.18.0.2"

// dnsBackup maps network service name → its DNS servers before we changed them
// (empty list = "Empty", i.e. use DHCP).
type dnsBackup map[string][]string

var dnsBackupFile string

func networksetup(args ...string) (string, error) {
	out, err := exec.Command("/usr/sbin/networksetup", args...).CombinedOutput()
	return string(out), err
}

func networkServices() []string {
	out, err := networksetup("-listallnetworkservices")
	if err != nil {
		return nil
	}
	var list []string
	for i, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if i == 0 || l == "" || strings.HasPrefix(l, "*") { // header, disabled services
			continue
		}
		list = append(list, l)
	}
	return list
}

func currentDNS(svc string) []string {
	out, err := networksetup("-getdnsservers", svc)
	if err != nil {
		return nil
	}
	var ips []string
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if l != "" && !strings.Contains(l, " ") {
			ips = append(ips, l)
		}
	}
	return ips
}

func setDNS(svc string, ips []string) {
	args := []string{"-setdnsservers", svc}
	if len(ips) == 0 {
		args = append(args, "Empty")
	} else {
		args = append(args, ips...)
	}
	if out, err := networksetup(args...); err != nil {
		log.Printf("networksetup %v: %v %s", args, err, out)
	}
}

// hijackSystemDNS points every network service at tunDNS, remembering the old servers.
func hijackSystemDNS() {
	if runtime.GOOS != "darwin" || os.Geteuid() != 0 {
		return
	}
	backup := dnsBackup{}
	if b, err := os.ReadFile(dnsBackupFile); err == nil {
		_ = json.Unmarshal(b, &backup) // keep the original values across restarts
	}
	for _, svc := range networkServices() {
		cur := currentDNS(svc)
		if _, ok := backup[svc]; !ok {
			if len(cur) == 1 && cur[0] == tunDNS {
				cur = nil
			}
			backup[svc] = cur
		}
		setDNS(svc, []string{tunDNS})
	}
	if b, err := json.Marshal(backup); err == nil {
		_ = os.WriteFile(dnsBackupFile, b, 0o600)
	}
}

// restoreSystemDNS undoes hijackSystemDNS (also after a crash: the backup is on disk).
func restoreSystemDNS() {
	if runtime.GOOS != "darwin" || os.Geteuid() != 0 {
		return
	}
	b, err := os.ReadFile(dnsBackupFile)
	if err != nil {
		return
	}
	var backup dnsBackup
	if json.Unmarshal(b, &backup) == nil {
		for svc, ips := range backup {
			setDNS(svc, ips)
		}
	}
	_ = os.Remove(dnsBackupFile)
}

func initDNSBackup(dataDir string) {
	dnsBackupFile = filepath.Join(dataDir, "dns-backup.json")
}
