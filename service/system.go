package service

import (
	"runtime"
	"time"
)

// SystemStatus is shown on the dashboard.
type SystemStatus struct {
	OS        string  `json:"os"`
	Arch      string  `json:"arch"`
	CPUCores  int     `json:"cpuCores"`
	CPUUsage  float64 `json:"cpuUsage"` // percent
	MemTotal  uint64  `json:"memTotal"`
	MemUsed   uint64  `json:"memUsed"`
	SwapTotal uint64  `json:"swapTotal"`
	SwapUsed  uint64  `json:"swapUsed"`
	DiskTotal uint64  `json:"diskTotal"`
	DiskUsed  uint64  `json:"diskUsed"`
	Uptime    uint64  `json:"uptime"` // seconds
	Load      string  `json:"load"`
	NetSent   uint64  `json:"netSent"`
	NetRecv   uint64  `json:"netRecv"`
	TCPConns  int     `json:"tcpConns"`
	UDPConns  int     `json:"udpConns"`
	Time      int64   `json:"time"`
}

// GetSystemStatus collects host metrics (best effort; fully supported on Linux).
func GetSystemStatus() SystemStatus {
	s := SystemStatus{
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		CPUCores: runtime.NumCPU(),
		Time:     time.Now().UnixMilli(),
	}
	fillPlatform(&s)
	return s
}
