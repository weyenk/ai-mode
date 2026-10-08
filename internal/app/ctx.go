package app

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// llama-server may run a role with a smaller context than the preset asks for:
// it caps the slot at the model's trained context (e.g. Qwen3-14B: 40960), or
// the preset was edited after the server started. The preset's ctx-size is then
// not what clients actually get, so we compare it against the n_ctx_slot the
// child process logged when it loaded.

var reLogCtxSlot = regexp.MustCompile(`^\[([\w.-]+):\d+\].*\bn_ctx_slot = (\d+)`)

// ctxMismatch is a role whose running context differs from its preset ctx-size.
type ctxMismatch struct {
	Role      string
	Preset    int // ctx-size from the ini
	Effective int // n_ctx_slot llama-server logged
}

// iniRoleCtx returns each role's configured ctx-size, falling back to the [*]
// default. Roles with no ctx-size anywhere are omitted.
func iniRoleCtx(text string) map[string]int {
	out := map[string]int{}
	section, def := "", 0
	seen := map[string]bool{}
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			if section != "*" {
				seen[section] = true
			}
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.ReplaceAll(strings.TrimSpace(k), "_", "-") {
		case "ctx-size", "c":
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil {
				continue
			}
			if section == "*" {
				def = n
			} else if section != "" {
				out[section] = n
			}
		}
	}
	if def > 0 {
		for s := range seen {
			if _, ok := out[s]; !ok {
				out[s] = def
			}
		}
	}
	return out
}

// effectiveCtxFromLog returns the most recent n_ctx_slot per role found in raw
// router log lines. Later lines win, so a restarted role reports its new value.
func effectiveCtxFromLog(lines []string) map[string]int {
	v := newLogView(true, "")
	out := map[string]int{}
	for _, l := range lines {
		s, ok := v.process(l)
		if !ok {
			continue
		}
		if m := reLogCtxSlot.FindStringSubmatch(s); m != nil {
			if n, err := strconv.Atoi(m[2]); err == nil {
				out[m[1]] = n
			}
		}
	}
	return out
}

// compareCtx lists roles (limited to `roles`) whose effective context is known
// and differs from the preset. Sorted by role.
func compareCtx(preset, effective map[string]int, roles []string) []ctxMismatch {
	var out []ctxMismatch
	for _, r := range roles {
		want, okW := preset[r]
		got, okG := effective[r]
		if okW && okG && want != got {
			out = append(out, ctxMismatch{Role: r, Preset: want, Effective: got})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Role < out[j].Role })
	return out
}

func (m ctxMismatch) String() string {
	if m.Effective < m.Preset {
		return fmt.Sprintf("%s: preset ctx-size %d but llama-server runs %d (usually the model's trained context caps it; "+
			"use YaRN rope scaling, a longer-context model, or lower ctx-size)", m.Role, m.Preset, m.Effective)
	}
	return fmt.Sprintf("%s: preset ctx-size %d but the running server uses %d (preset changed since load; run: ai-mode restart)",
		m.Role, m.Preset, m.Effective)
}

// profileCtx returns the effective contexts logged for the profile and the
// mismatches among the given (currently loaded) roles.
func profileCtx(p Profile, roles []string) (effective map[string]int, bad []ctxMismatch) {
	data, err := os.ReadFile(p.Ini)
	if err != nil {
		return nil, nil
	}
	logData, err := os.ReadFile(p.LogFile())
	if err != nil {
		return nil, nil
	}
	effective = effectiveCtxFromLog(strings.Split(string(logData), "\n"))
	return effective, compareCtx(iniRoleCtx(string(data)), effective, roles)
}

// loadedRoles returns catalog roles that are configured in the ini and loaded.
func loadedRoles(p Profile, cat catalog) []string {
	configured := iniSectionsFile(p.Ini)
	var out []string
	for id, m := range cat {
		if contains(configured, id) && modelStatus(m) == "loaded" {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// presetChangedSince reports whether the ini was modified after the server
// started (startedAt is State.StartedAt).
func presetChangedSince(iniMod time.Time, startedAt string) bool {
	t, err := time.Parse("2006-01-02T15:04:05.000000-07:00", startedAt)
	if err != nil {
		return false
	}
	return iniMod.After(t)
}

func warnPresetChanged(p Profile, st State) {
	if info, err := os.Stat(p.Ini); err == nil && presetChangedSince(info.ModTime(), st.StartedAt) {
		fmt.Fprintf(os.Stderr, "Note: %s was edited after the server started, so the running models may not match it. "+
			"Apply it with: ai-mode restart\n", p.Ini)
	}
}
