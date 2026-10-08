package app

import (
	"os"
	"path/filepath"
	"strings"
)

const appName = "ai-mode"

var homeDir = func() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return h
}()

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func configDir() string {
	return envOr("AI_MODE_CONFIG", filepath.Join(homeDir, ".config", "ai-mode"))
}

func stateDir() string {
	return envOr("AI_MODE_STATE", filepath.Join(homeDir, ".local", "state", "ai-mode"))
}

func stateFile() string     { return filepath.Join(stateDir(), "state.json") }
func activeEnvFile() string { return filepath.Join(configDir(), "active.env") }
func logDir() string        { return filepath.Join(stateDir(), "logs") }

func ensureDirs() {
	for _, d := range []string{configDir(), stateDir(), logDir()} {
		_ = os.MkdirAll(d, 0o755)
	}
}

func expandHome(p string) string {
	if p == "~" {
		return homeDir
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(homeDir, p[2:])
	}
	return p
}

// configValue reads KEY from ~/.config/ai-mode/config (KEY=value lines).
func configValue(key string) string {
	data, err := os.ReadFile(filepath.Join(configDir(), "config"))
	if err != nil {
		return ""
	}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(k) == key {
			return strings.Trim(strings.TrimSpace(v), "\"'")
		}
	}
	return ""
}

func hasIni(dir string) bool {
	m, _ := filepath.Glob(filepath.Join(dir, "*.ini"))
	return len(m) > 0
}

// presetsDir resolves: $AI_MODE_PRESETS, config file, <repo>/presets next to the
// (symlink-resolved) binary's parent dir, then ~/models/presets.
func presetsDir() string {
	if p := os.Getenv("AI_MODE_PRESETS"); p != "" {
		return expandHome(p)
	}
	if p := configValue("AI_MODE_PRESETS"); p != "" {
		return expandHome(p)
	}
	if exe, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			sib := filepath.Join(filepath.Dir(filepath.Dir(real)), "presets")
			if hasIni(sib) {
				return sib
			}
		}
	}
	return filepath.Join(homeDir, "models", "presets")
}

func agentsDir() string { return filepath.Join(presetsDir(), "agents") }
