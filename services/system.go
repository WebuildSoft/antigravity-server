package services

import (
	"encoding/json"
	"net"
	"os"
	"runtime"
	"time"

	"antigravity-bridge/config"
)

type NetworkAddress struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

type VersionData struct {
	Version    string `json:"version"`
	Build      int    `json:"build"`
	ReleasedAt string `json:"releasedAt"`
}

type ServerStatus struct {
	Status           string           `json:"status"`
	Name             string           `json:"name"`
	Version          string           `json:"version"`
	Hostname         string           `json:"hostname"`
	Platform         string           `json:"platform"`
	IPs              []NetworkAddress `json:"ips"`
	ActiveSessions   int              `json:"activeSessions"`
	AgyBin           string           `json:"agyBin"`
	WebDir           string           `json:"webDir"`
	TelemetryEnabled bool             `json:"telemetryEnabled"`
	Timestamp        string           `json:"timestamp"`
}

func GetNetworkAddresses() []NetworkAddress {
	var addresses []NetworkAddress
	ifaces, err := net.Interfaces()
	if err != nil {
		return addresses
	}

	for _, i := range ifaces {
		if (i.Flags & net.FlagUp) == 0 {
			continue
		}
		addrs, err := i.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
				addresses = append(addresses, NetworkAddress{
					Name:    i.Name,
					Address: ip.String(),
				})
			}
		}
	}
	return addresses
}

func LoadVersionData() VersionData {
	data, err := os.ReadFile(config.AppConfig.VersionFile)
	if err == nil {
		var vd VersionData
		if json.Unmarshal(data, &vd) == nil {
			return vd
		}
	}
	return VersionData{
		Version:    "1.4.0",
		Build:      24,
		ReleasedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

func GetServerStatus(activeSessions int) ServerStatus {
	hostname, _ := os.Hostname()
	return ServerStatus{
		Status:           "online",
		Name:             "Antigravity Mobile Bridge (Go)",
		Version:          "2.0.0",
		Hostname:         hostname,
		Platform:         runtime.GOOS,
		IPs:              GetNetworkAddresses(),
		ActiveSessions:   activeSessions,
		AgyBin:           config.AppConfig.AgyBin,
		WebDir:           config.AppConfig.WebDir,
		TelemetryEnabled: config.AppConfig.TelemetryEnabled,
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
	}
}
