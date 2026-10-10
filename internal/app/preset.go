package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// parseKV parses simple `key = value` text (.mode files). Lines starting with
// # or ; are comments. Keys are lowercased with _ → -.
func parseKV(text string) map[string]string {
	out := map[string]string{}
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(k)), "_", "-")
		out[key] = strings.Trim(strings.TrimSpace(v), "\"'")
	}
	return out
}

func parseModeFile(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	return parseKV(string(data))
}

// iniSections returns [section] names in file order, excluding the [*] defaults.
func iniSections(text string) []string {
	var names []string
	for _, raw := range strings.Split(text, "\n") {
		s := strings.TrimSpace(raw)
		if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") && s != "[*]" {
			names = append(names, s[1:len(s)-1])
		}
	}
	return names
}

func iniSectionsFile(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return iniSections(string(data))
}

type Profile struct {
	Name        string
	Ini         string
	Mode        string // path to .mode, "" if absent
	Description string
	Host        string
	Port        int
	ModelsMax   int
}

func (p Profile) BaseURL() string { return fmt.Sprintf("http://%s:%d/v1", p.ClientHost(), p.Port) }

// ClientHost is the address clients should dial: a wildcard bind (0.0.0.0 / ::)
// listens on every interface, but isn't a usable destination, so use loopback.
func (p Profile) ClientHost() string {
	if p.Host == "0.0.0.0" || p.Host == "::" {
		return "127.0.0.1"
	}
	return p.Host
}
func (p Profile) LogFile() string { return filepath.Join(logDir(), p.Name+".log") }

func atoi(s string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return n
}

func resolvePath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}

func discoverProfiles() []Profile {
	root := presetsDir()
	inis, _ := filepath.Glob(filepath.Join(root, "*.ini"))
	sort.Strings(inis)

	var profiles []Profile
	used := map[int]bool{}
	for _, ini := range inis {
		name := strings.TrimSuffix(filepath.Base(ini), ".ini")
		modePath := filepath.Join(root, name+".mode")
		meta := parseModeFile(modePath)

		port := atoi(meta["port"], 0)
		if port <= 0 {
			port = 8080 + len(profiles)
			for used[port] {
				port++
			}
		}
		used[port] = true

		p := Profile{
			Name:        name,
			Ini:         resolvePath(ini),
			Description: meta["description"],
			Host:        "127.0.0.1",
			Port:        port,
			ModelsMax:   atoi(meta["models-max"], 2),
		}
		if h := meta["host"]; h != "" {
			p.Host = h
		}
		if _, err := os.Stat(modePath); err == nil {
			p.Mode = resolvePath(modePath)
		}
		profiles = append(profiles, p)
	}
	return profiles
}

func getProfile(name string) (Profile, bool) {
	name = strings.TrimSpace(name)
	for _, p := range discoverProfiles() {
		if p.Name == name {
			return p, true
		}
	}
	return Profile{}, false
}

func (p Profile) modeMeta() map[string]string {
	if p.Mode == "" {
		return map[string]string{}
	}
	return parseModeFile(p.Mode)
}

// warmList: explicit warm= in .mode, else chief/jev/architect present in the ini.
func (p Profile) warmList() []string {
	if raw, ok := p.modeMeta()["warm"]; ok {
		var out []string
		for _, s := range strings.Split(raw, ",") {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	have := map[string]bool{}
	for _, s := range iniSectionsFile(p.Ini) {
		have[s] = true
	}
	var out []string
	for _, n := range []string{"chief", "jev", "architect"} {
		if have[n] {
			out = append(out, n)
		}
	}
	return out
}

// ---- agents/<role>.md ------------------------------------------------------

// parseAgent splits `---` frontmatter (flat key: value) from the body.
func parseAgent(text string) (map[string]string, string) {
	meta := map[string]string{}
	if strings.HasPrefix(text, "---") {
		parts := strings.SplitN(text, "---", 3)
		if len(parts) >= 3 {
			for _, raw := range strings.Split(parts[1], "\n") {
				line := strings.TrimSpace(raw)
				k, v, ok := strings.Cut(line, ":")
				if line == "" || !ok {
					continue
				}
				meta[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), "\"'")
			}
			return meta, strings.TrimLeft(parts[2], "\n")
		}
	}
	return meta, text
}

func parseAgentFile(path string) (map[string]string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	meta, body := parseAgent(string(data))
	return meta, body, nil
}

// resolveAgentFile follows alias_of to the canonical agent file.
func resolveAgentFile(role string, seen map[string]bool) (string, error) {
	if seen[role] {
		return "", fmt.Errorf("agent alias cycle involving '%s'", role)
	}
	path := filepath.Join(agentsDir(), role+".md")
	meta, _, err := parseAgentFile(path)
	if err != nil {
		return path, nil
	}
	target := strings.TrimSpace(meta["alias_of"])
	if target == "" || target == role {
		return path, nil
	}
	next := map[string]bool{role: true}
	for k := range seen {
		next[k] = true
	}
	return resolveAgentFile(target, next)
}

func listAgentRoles() []string {
	files, _ := filepath.Glob(filepath.Join(agentsDir(), "*.md"))
	sort.Strings(files) // by filename: "coder-xl.md" sorts before "coder.md"
	var roles []string
	for _, f := range files {
		if strings.EqualFold(filepath.Base(f), "readme.md") {
			continue
		}
		roles = append(roles, strings.TrimSuffix(filepath.Base(f), ".md"))
	}
	return roles
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
