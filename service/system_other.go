//go:build !linux

package service

// fillPlatform is a no-op outside Linux; the panel is meant to run on a Linux server.
func fillPlatform(s *SystemStatus) {}
