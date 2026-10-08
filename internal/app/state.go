package app

import (
	"encoding/json"
	"os"
	"time"
)

// State is persisted to state.json. Keys are stable across versions.
type State struct {
	Profile      string `json:"profile,omitempty"`
	PID          int    `json:"pid,omitempty"`
	Port         int    `json:"port,omitempty"`
	Host         string `json:"host,omitempty"`
	Ini          string `json:"ini,omitempty"`
	StartedAt    string `json:"started_at,omitempty"`
	Log          string `json:"log,omitempty"`
	Debug        bool   `json:"debug,omitempty"` // debug proxy fronting the server
	ProxyPID     int    `json:"proxy_pid,omitempty"`
	UpstreamPort int    `json:"upstream_port,omitempty"` // llama-server's port in debug mode
}

func loadState() State {
	ensureDirs()
	var st State
	data, err := os.ReadFile(stateFile())
	if err != nil {
		return st
	}
	_ = json.Unmarshal(data, &st)
	return st
}

func saveState(st State) {
	ensureDirs()
	data, _ := json.MarshalIndent(st, "", "  ")
	_ = os.WriteFile(stateFile(), append(data, '\n'), 0o644)
}

func nowISO() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000000-07:00")
}
