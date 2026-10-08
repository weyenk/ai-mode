package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

func cmdList(args []string) int {
	fs := newFlags("list")
	if _, code, ok := parse(fs, args); !ok {
		return code
	}
	profiles := discoverProfiles()
	active := loadState().Profile
	root := presetsDir()
	if len(profiles) == 0 {
		fmt.Printf("No profiles found in %s\n", root)
		fmt.Println("Add a <name>.ini (optional <name>.mode for port/description).")
		return 1
	}
	fmt.Printf("Presets: %s\n\n", root)
	for _, p := range profiles {
		mark := " "
		if p.Name == active {
			mark = "*"
		}
		desc := ""
		if p.Description != "" {
			desc = "  — " + p.Description
		}
		mode := "auto-port"
		if p.Mode != "" {
			mode = "mode"
		}
		fmt.Printf(" %s %-20s :%-5d [%s]%s\n", mark, p.Name, p.Port, mode, desc)
	}
	fmt.Println("\n* = active profile (ai-mode use)")
	return 0
}

func cmdWhich(args []string) int {
	fs := newFlags("which")
	asJSON := fs.Bool("json", false, "JSON output")
	if _, code, ok := parse(fs, args); !ok {
		return code
	}
	st := loadState()
	if st.Profile == "" {
		fmt.Println("No active profile. Run: ai-mode use <name>")
		return 1
	}
	p, found := getProfile(st.Profile)
	pid := managedPID(st)
	running := pid != 0 && found && portOpen(p.Host, p.Port) && (!st.Debug || pidAlive(st.ProxyPID))

	if *asJSON {
		var pidV, baseV, portV, hostV, iniV, startedV any
		if pid != 0 {
			pidV = pid
		}
		startedV = nilIfEmpty(st.StartedAt)
		if found {
			baseV, portV, hostV, iniV = p.BaseURL(), p.Port, p.Host, p.Ini
		} else {
			portV, hostV, iniV = nilIfZero(st.Port), nilIfEmpty(st.Host), nilIfEmpty(st.Ini)
		}
		out := orderedJSON{
			{"profile", st.Profile}, {"pid", pidV}, {"running", running},
			{"base_url", baseV}, {"port", portV}, {"host", hostV},
			{"ini", iniV}, {"started_at", startedV},
		}
		fmt.Print(prettyJSON(out))
		return 0
	}

	fmt.Printf("profile:  %s\n", st.Profile)
	if found {
		fmt.Printf("base_url: %s\nport:     %d\nini:      %s\n", p.BaseURL(), p.Port, p.Ini)
		if p.Description != "" {
			fmt.Printf("about:    %s\n", p.Description)
		}
	}
	if pid != 0 {
		fmt.Printf("pid:      %d\n", pid)
	} else {
		fmt.Println("pid:      —")
	}
	if st.Debug {
		state := "running"
		if !pidAlive(st.ProxyPID) {
			state = "DOWN (clients cannot reach the models; run: ai-mode use " + st.Profile + " --debug --force)"
		}
		fmt.Printf("mode:     DEBUG — proxy :%d → llama-server :%d, proxy %s\n", st.Port, st.UpstreamPort, state)
	}
	if running {
		fmt.Println("server:   running")
	} else {
		fmt.Println("server:   stopped")
	}
	if st.StartedAt != "" {
		fmt.Printf("started:  %s\n", st.StartedAt)
	}
	if running {
		return 0
	}
	return 2
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nilIfZero(n int) any {
	if n == 0 {
		return nil
	}
	return n
}

// orderedJSON marshals as a JSON object preserving key order.
type orderedJSON []struct {
	K string
	V any
}

func (o orderedJSON) MarshalJSON() ([]byte, error) {
	var b strings.Builder
	b.WriteByte('{')
	for i, kv := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(kv.K)
		v, err := json.Marshal(kv.V)
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return []byte(b.String()), nil
}

type useOpts struct {
	force       bool
	timeout     float64
	noWarm      bool
	debug       bool
	warmTimeout float64
}

func addUseFlags(fs interface {
	BoolVar(*bool, string, bool, string)
	Float64Var(*float64, string, float64, string)
}, o *useOpts, withForce bool) {
	if withForce {
		fs.BoolVar(&o.force, "force", false, "Restart even if already active")
	}
	fs.Float64Var(&o.timeout, "timeout", 60, "Health wait seconds")
	fs.BoolVar(&o.noWarm, "no-warm", false, "Skip prefetch after router is healthy")
	fs.BoolVar(&o.debug, "debug", false, "Front the server with a logging proxy that records all model traffic (Qwen, curl, …)")
	fs.Float64Var(&o.warmTimeout, "warm-timeout", 600, "Seconds to wait per model during warm (HF download)")
}

func maybeWarmAfterUse(p Profile, o useOpts) {
	if o.noWarm {
		return
	}
	names := p.warmList()
	if len(names) == 0 {
		return
	}
	fmt.Println()
	if warmModels(p, names, secs(o.warmTimeout)) != 0 {
		fmt.Fprintln(os.Stderr, "Warm incomplete — first requests may still download HF weights.")
	}
}

func cmdUse(args []string) int {
	fs := newFlags("use")
	var o useOpts
	addUseFlags(fs, &o, true)
	pos, code, ok := parse(fs, args)
	if !ok {
		return code
	}
	if len(pos) != 1 {
		return fail("usage: ai-mode use <profile> [--force] [--timeout S] [--no-warm] [--warm-timeout S]")
	}
	return lifecycle("use", pos[0], func() int { return useProfile(pos[0], o) })
}

func useProfile(name string, o useOpts) int {
	p, found := getProfile(name)
	if !found {
		names := []string{}
		for _, q := range discoverProfiles() {
			names = append(names, q.Name)
		}
		avail := "(none)"
		if len(names) > 0 {
			avail = strings.Join(names, ", ")
		}
		fmt.Printf("Unknown profile: %s\nAvailable: %s\n", name, avail)
		return 1
	}

	st := loadState()
	pid := managedPID(st)

	if !o.force && st.Profile == p.Name && pid != 0 && st.Debug == o.debug && portOpen(p.Host, p.Port) &&
		(!o.debug || pidAlive(st.ProxyPID)) {
		if _, healthy := getJSON(p.BaseURL()+"/models", 2*time.Second); healthy {
			fmt.Printf("Already using %s at %s\n", p.Name, p.BaseURL())
			warnPresetChanged(p, st)
			writeActiveEnv(p)
			maybeWarmAfterUse(p, o)
			return 0
		}
	}

	if pid != 0 || st.PID != 0 || st.ProxyPID != 0 {
		prev := st.Profile
		if prev == "" {
			prev = "previous"
		}
		fmt.Printf("Stopping %s (pid %d)…\n", prev, st.PID)
		if !stopManaged(&st, 8*time.Second) {
			fmt.Fprintln(os.Stderr, "Failed to stop previous llama-server; try: ai-mode stop --force")
			return 1
		}
		st = loadState()
	}

	if portOpen(p.Host, p.Port) {
		fmt.Fprintf(os.Stderr, "Port %d is busy but not owned by ai-mode.\nFree it or edit %s.mode\n", p.Port, p.Name)
		return 1
	}

	// Debug mode: llama-server hides on port+10000 and the proxy owns the public port.
	serverPort := p.Port
	if o.debug {
		serverPort = upstreamPort(p)
		fmt.Printf("Starting %s in DEBUG mode: proxy %s:%d → llama-server :%d\n", p.Name, p.Host, p.Port, serverPort)
	} else {
		fmt.Printf("Starting %s on %s…\n", p.Name, p.BaseURL())
	}
	newPID, err := startProfile(p, serverPort)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	newState := State{
		Profile: p.Name, PID: newPID, Port: p.Port, Host: p.Host,
		Ini: p.Ini, StartedAt: nowISO(), Log: p.LogFile(),
	}
	if o.debug {
		newState.Debug, newState.UpstreamPort = true, serverPort
	}
	saveState(newState)
	writeActiveEnv(p)

	fmt.Printf("pid %d; waiting for health…\n", newPID)
	healthy := false
	if o.debug {
		if waitHealthyAt(p.UpstreamBaseURL(), secs(o.timeout)) {
			proxyPID, perr := startProxy(p)
			if perr != nil {
				fmt.Fprintf(os.Stderr, "llama-server is up but the debug proxy failed to start: %v\n", perr)
				return 2
			}
			newState.ProxyPID = proxyPID
			saveState(newState)
			healthy = waitHealthy(p, 30*time.Second)
			if !healthy {
				fmt.Fprintf(os.Stderr, "debug proxy (pid %d) is not answering on %s; see %s\n", proxyPID, p.BaseURL(), p.ProxyLogFile())
				return 2
			}
		}
	} else {
		healthy = waitHealthy(p, secs(o.timeout))
	}
	if healthy {
		fmt.Printf("Active: %s → %s\n", p.Name, p.BaseURL())
		if data, ok := getJSON(p.BaseURL()+"/models", 3*time.Second); ok {
			var ids []string
			items, _ := data["data"].([]any)
			for _, it := range items {
				if m := asMap(it); m != nil {
					ids = append(ids, asString(m["id"]))
				}
			}
			if len(ids) > 0 {
				shown, more := ids, ""
				if len(ids) > 12 {
					shown, more = ids[:12], "…"
				}
				fmt.Printf("Models (%d): %s%s\n", len(ids), strings.Join(shown, ", "), more)
			}
		}
		fmt.Println(`Env:    eval "$(ai-mode env)"`)
		fmt.Println("Logs:   ai-mode logs")
		if o.debug {
			fmt.Printf("DEBUG:  all chat traffic on :%d is recorded → ai-mode events -f / stats / review (proxy log: %s)\n", p.Port, p.ProxyLogFile())
			fmt.Printf("        back to normal: ai-mode use %s\n", p.Name)
		}
		maybeWarmAfterUse(p, o)
		return 0
	}
	fmt.Fprintf(os.Stderr, "Server started (pid %d) but /v1/models not healthy yet.\nCheck: ai-mode logs -f\nLog:   %s\n", newPID, p.LogFile())
	return 2
}

func cmdStop(args []string) int {
	fs := newFlags("stop")
	force := fs.Bool("force", false, "SIGKILL if needed")
	if _, code, ok := parse(fs, args); !ok {
		return code
	}
	return lifecycle("stop", loadState().Profile, func() int { return stopCmd(*force) })
}

func stopCmd(force bool) int {
	st := loadState()
	if st.PID == 0 && st.Profile == "" {
		fmt.Println("Nothing to stop.")
		return 0
	}
	name := st.Profile
	if name == "" {
		name = "?"
	}
	pid := st.PID
	fmt.Printf("Stopping %s (pid %d)…\n", name, pid)
	wait := 8 * time.Second
	if force {
		wait = 12 * time.Second
	}
	stopped := stopManaged(&st, wait)
	if !stopped && force && pid != 0 {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		st = loadState()
		st.PID = 0
		saveState(st)
		stopped = true
	}
	if stopped {
		fmt.Println("Stopped.")
		return 0
	}
	fmt.Println("Failed to stop.")
	return 1
}

func cmdRestart(args []string) int {
	fs := newFlags("restart")
	var o useOpts
	addUseFlags(fs, &o, false)
	pos, code, ok := parse(fs, args)
	if !ok {
		return code
	}
	name := loadState().Profile
	if len(pos) > 0 {
		name = pos[0]
	}
	if name == "" {
		fmt.Println("No profile to restart. Use: ai-mode use <name>")
		return 1
	}
	return lifecycle("restart", name, func() int {
		o.debug = o.debug || loadState().Debug // restart keeps the current mode
		stopCmd(false)
		o.force = true
		return useProfile(name, o)
	})
}

func cmdEnv(args []string) int {
	fs := newFlags("env")
	pos, code, ok := parse(fs, args)
	if !ok {
		return code
	}
	name := loadState().Profile
	if len(pos) > 0 {
		name = pos[0]
	}
	if name == "" {
		fmt.Fprintln(os.Stderr, "# no active profile")
		return 1
	}
	p, found := getProfile(name)
	if !found {
		fmt.Fprintf(os.Stderr, "# unknown profile: %s\n", name)
		return 1
	}
	writeActiveEnv(p)
	fmt.Printf("export AI_MODE_PROFILE=%q\n", p.Name)
	fmt.Printf("export AI_MODE_PORT=\"%d\"\n", p.Port)
	fmt.Printf("export AI_MODE_HOST=%q\n", p.Host)
	fmt.Printf("export AI_MODE_BASE_URL=%q\n", p.BaseURL())
	fmt.Printf("export OPENAI_BASE_URL=%q\n", p.BaseURL())
	fmt.Println(`export OPENAI_API_KEY="${OPENAI_API_KEY:-local}"`)
	return 0
}

func cmdURL(args []string) int {
	fs := newFlags("url")
	if _, code, ok := parse(fs, args); !ok {
		return code
	}
	p, code := activeProfile("", "")
	if code != 0 {
		return code
	}
	fmt.Println(p.BaseURL())
	return 0
}

func cmdWarm(args []string) int {
	fs := newFlags("warm")
	profile := fs.String("profile", "", "Profile (default: active)")
	all := fs.Bool("all", false, "Warm every ini section (except [*])")
	wt := fs.Float64("warm-timeout", 600, "Seconds to wait per model (HF download)")
	models, code, ok := parse(fs, args)
	if !ok {
		return code
	}
	p, code := activeProfile(*profile, "Use: ai-mode use <name> or ai-mode warm --profile <name>")
	if code != 0 {
		return code
	}
	var names []string
	switch {
	case *all:
		if len(models) > 0 {
			fmt.Fprintln(os.Stderr, "Ignoring positional models (--all warms every ini section).")
		}
		names = iniSectionsFile(p.Ini)
	case len(models) > 0:
		names = models
	default:
		names = p.warmList()
	}
	return lifecycle("warm", p.Name, func() int { return warmModels(p, names, secs(*wt)) })
}

func cmdModels(args []string) int {
	fs := newFlags("models")
	if _, code, ok := parse(fs, args); !ok {
		return code
	}
	p, code := activeProfile("", "")
	if code != 0 {
		return code
	}
	data, ok := getJSON(p.BaseURL()+"/models", 3*time.Second)
	if !ok {
		return fail("API not reachable at %s", p.BaseURL())
	}
	items, _ := data["data"].([]any)
	for _, it := range items {
		id := "?"
		if m := asMap(it); m != nil && m["id"] != nil {
			id = asString(m["id"])
		}
		fmt.Println(id)
	}
	fmt.Fprintln(os.Stderr, "---\nconfigured in preset:")
	for _, s := range iniSectionsFile(p.Ini) {
		fmt.Fprintf(os.Stderr, "  %s\n", s)
	}
	return 0
}

func cmdInit(args []string) int {
	fs := newFlags("init")
	port := fs.Int("port", 0, "Port (default: next free from 8080)")
	modelsMax := fs.Int("models-max", 2, "models-max")
	desc := fs.String("description", "", "Description")
	withIni := fs.Bool("with-ini", false, "Also create a starter .ini")
	force := fs.Bool("force", false, "Overwrite existing .mode")
	pos, code, ok := parse(fs, args)
	if !ok {
		return code
	}
	if len(pos) != 1 {
		return fail("usage: ai-mode init <profile> [--port N] [--models-max N] [--description S] [--with-ini] [--force]")
	}
	name := pos[0]
	root := presetsDir()
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fail("%v", err)
	}
	ini := filepath.Join(root, name+".ini")
	mode := filepath.Join(root, name+".mode")

	if *withIni && !isFile(ini) {
		body := fmt.Sprintf(`; %s — llama-server router preset
version = 1

[*]
jinja = 1
flash-attn = on
n-gpu-layers = 99
load-mode = mmap

; [example]
; hf = unsloth/Qwen3-4B-GGUF:Q4_K_M
; ctx-size = 16384
`, name)
		if err := os.WriteFile(ini, []byte(body), 0o644); err != nil {
			return fail("%v", err)
		}
		fmt.Printf("Created %s\n", ini)
	}

	if isFile(mode) && !*force {
		fmt.Printf("Already exists: %s (use --force to overwrite)\n", mode)
		return 1
	}

	used := map[int]bool{}
	for _, p := range discoverProfiles() {
		if p.Name != name {
			used[p.Port] = true
		}
	}
	pt := *port
	if pt <= 0 {
		pt = 8080
		for used[pt] {
			pt++
		}
	}
	d := *desc
	if d == "" {
		d = name
	}
	body := fmt.Sprintf(`# ai-mode metadata for profile "%s"
description = %s
port = %d
models-max = %d
host = 127.0.0.1
# warm = chief,jev,architect
`, name, d, pt, *modelsMax)
	if err := os.WriteFile(mode, []byte(body), 0o644); err != nil {
		return fail("%v", err)
	}
	fmt.Printf("Created %s\nEdit models in: %s\n", mode, ini)
	return 0
}

func cmdAgents(args []string) int {
	fs := newFlags("agents")
	filter := fs.String("profile", "", "Filter by profile name")
	if _, code, ok := parse(fs, args); !ok {
		return code
	}
	adir := agentsDir()
	if !isDir(adir) {
		fmt.Printf("No agents directory at %s\n", adir)
		return 1
	}
	roles := listAgentRoles()
	if len(roles) == 0 {
		fmt.Printf("No agent guides in %s\n", adir)
		return 1
	}
	roleProfiles := map[string][]string{}
	for _, p := range discoverProfiles() {
		for _, r := range iniSectionsFile(p.Ini) {
			roleProfiles[r] = append(roleProfiles[r], p.Name)
		}
	}
	fmt.Printf("Agents: %s\n\n", adir)
	for _, stem := range roles {
		meta, _, err := parseAgentFile(filepath.Join(adir, stem+".md"))
		if err != nil {
			continue
		}
		role := meta["role"]
		if role == "" {
			role = stem
		}
		profile := meta["profile"]
		if profile == "" {
			profile = strings.Join(roleProfiles[role], ",")
		}
		if profile == "" {
			profile = "?"
		}
		if *filter != "" && profile != *filter && !contains(roleProfiles[role], *filter) {
			continue
		}
		line := fmt.Sprintf("  %-18s profile=%s", role, profile)
		if t := meta["trigger"]; t != "" {
			line += "  trigger=" + t
		}
		if _, ok := roleProfiles[role]; !ok {
			line += "  (no matching ini section)"
		}
		fmt.Println(line)
	}
	return 0
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// agentForRole loads and resolves agents/<role>.md, printing errors itself.
func agentForRole(role string) (path string, meta map[string]string, body string, code int) {
	if !isFile(filepath.Join(agentsDir(), role+".md")) {
		fmt.Fprintf(os.Stderr, "No agent guide for '%s'\n", role)
		if avail := listAgentRoles(); len(avail) > 0 {
			sort.Strings(avail)
			fmt.Fprintf(os.Stderr, "Available: %s\n", strings.Join(avail, ", "))
		}
		return "", nil, "", 1
	}
	path, err := resolveAgentFile(role, nil)
	if err != nil {
		return "", nil, "", fail("%v", err)
	}
	if !isFile(path) {
		return "", nil, "", fail("No agent guide for alias target of '%s'", role)
	}
	meta, body, err = parseAgentFile(path)
	if err != nil {
		return "", nil, "", fail("%v", err)
	}
	return path, meta, body, 0
}

func cmdPrompt(args []string) int {
	fs := newFlags("prompt")
	asJSON := fs.Bool("json", false, "Include frontmatter + body as JSON")
	raw := fs.Bool("raw", false, "Print full markdown including frontmatter")
	pos, code, ok := parse(fs, args)
	if !ok {
		return code
	}
	if len(pos) != 1 {
		return fail("usage: ai-mode prompt <role> [--json] [--raw]")
	}
	role := pos[0]
	path, meta, body, code := agentForRole(role)
	if code != 0 {
		return code
	}
	switch {
	case *asJSON:
		fmt.Print(prettyJSON(orderedJSON{{"role", role}, {"path", path}, {"meta", meta}, {"prompt", body}}))
	case *raw:
		data, _ := os.ReadFile(path)
		os.Stdout.Write(data)
		if len(data) > 0 && data[len(data)-1] != '\n' {
			fmt.Println()
		}
	default:
		io.WriteString(os.Stdout, body)
		if !strings.HasSuffix(body, "\n") {
			fmt.Println()
		}
	}
	return 0
}
