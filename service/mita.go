// Package service contains the business logic that drives the mita server.
package service

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Mita wraps the official `mita` command line tool.
type Mita struct {
	Bin string
}

func (m Mita) bin() string {
	if m.Bin == "" {
		return "mita"
	}
	return m.Bin
}

// Installed reports whether the mita binary can be found.
func (m Mita) Installed() bool {
	_, err := exec.LookPath(m.bin())
	return err == nil
}

func (m Mita) run(timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, m.bin(), args...)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return text, fmt.Errorf("mita %s: timeout", strings.Join(args, " "))
	}
	if err != nil {
		if text != "" {
			return text, fmt.Errorf("mita %s: %v: %s", strings.Join(args, " "), err, text)
		}
		return text, fmt.Errorf("mita %s: %v", strings.Join(args, " "), err)
	}
	return text, nil
}

// Version returns `mita version`.
func (m Mita) Version() (string, error) { return m.run(5*time.Second, "version") }

// Status returns the raw `mita status` output and whether the proxy is running.
func (m Mita) Status() (string, bool, error) {
	out, err := m.run(5*time.Second, "status")
	if err != nil {
		return out, false, err
	}
	return out, strings.Contains(strings.ToUpper(out), "RUNNING"), nil
}

// Start starts the mita proxy service.
func (m Mita) Start() (string, error) { return m.run(30*time.Second, "start") }

// Stop stops the mita proxy service.
func (m Mita) Stop() (string, error) { return m.run(30*time.Second, "stop") }

// Reload makes a running mita server pick up the new configuration.
func (m Mita) Reload() (string, error) { return m.run(30*time.Second, "reload") }

// ApplyConfig runs `mita apply config <file>`.
func (m Mita) ApplyConfig(path string) (string, error) {
	return m.run(15*time.Second, "apply", "config", path)
}

// DeleteUser runs `mita delete user <name>`.
func (m Mita) DeleteUser(name string) (string, error) {
	return m.run(15*time.Second, "delete", "user", name)
}

// Describe runs `mita describe config` (current effective config, as JSON).
func (m Mita) Describe() (string, error) { return m.run(10*time.Second, "describe", "config") }

// GetUsers runs `mita get users`.
func (m Mita) GetUsers() (string, error) { return m.run(10*time.Second, "get", "users") }

// GetMetrics runs `mita get metrics`.
func (m Mita) GetMetrics() (string, error) { return m.run(10*time.Second, "get", "metrics") }
