package app

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func fileContains(path, needle string) bool {
	data, err := os.ReadFile(path)
	return err == nil && bytes.Contains(data, []byte(needle))
}

func cmdDoctor(args []string) int {
	fs := newFlags("doctor")
	if _, code, ok := parse(fs, args); !ok {
		return code
	}
	ensureDirs()
	root := presetsDir()
	profiles := discoverProfiles()
	st := loadState()
	llama := findLlamaServer()
	var issues []string
	flag := func(ok bool, yes, no string) string {
		if ok {
			return yes
		}
		return no
	}

	fmt.Printf("%s doctor\n%s\n", appName, strings.Repeat("=", 40))
	fmt.Printf("presets dir:  %s%s\n", root, flag(isDir(root), "  OK", "  MISSING"))
	if !isDir(root) {
		issues = append(issues, fmt.Sprintf("Create %s and add <name>.ini profiles", root))
	}

	if llama == "" {
		fmt.Println("llama-server: NOT FOUND")
		issues = append(issues, "Install llama.cpp (brew install llama.cpp)")
	} else {
		fmt.Printf("llama-server: %s\n", llama)
		out, _ := exec.Command(llama, "--version").CombinedOutput()
		if line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n"); line != "" {
			fmt.Printf("version:      %s\n", line)
		}
		// Decision models (Jev) need /v1/systemone in the server binary.
		real := resolvePath(llama)
		target := real
		if impl := filepath.Join(filepath.Dir(filepath.Dir(real)), "lib", "libllama-server-impl.dylib"); isFile(impl) {
			target = impl
		}
		if fileContains(target, "/v1/systemone") {
			fmt.Println("/v1/systemone: OK (Jev/Kev ready)")
		} else {
			fmt.Println("/v1/systemone: MISSING")
			issues = append(issues, "llama-server lacks /v1/systemone — upgrade llama.cpp (brew install --HEAD llama.cpp) for jev/Kev-4B")
		}
	}

	_, statErr := os.Stat(stateFile())
	_, envErr := os.Stat(activeEnvFile())
	fmt.Printf("config:       %s\n", configDir())
	fmt.Printf("state:        %s%s\n", stateFile(), flag(statErr == nil, "  (present)", "  (empty)"))
	fmt.Printf("active.env:   %s%s\n\n", activeEnvFile(), flag(envErr == nil, "  OK", "  (none yet)"))

	if len(profiles) == 0 {
		issues = append(issues, "No *.ini profiles discovered")
	} else {
		fmt.Printf("Profiles (%d):\n", len(profiles))
		ports := map[int]string{}
		for _, p := range profiles {
			clash := ""
			if other, dup := ports[p.Port]; dup {
				clash = "PORT CLASH with " + other
				issues = append(issues, fmt.Sprintf("Port clash: %s and %s both use %d", p.Name, other, p.Port))
			}
			ports[p.Port] = p.Name
			sections := iniSectionsFile(p.Ini)
			list := "(none)"
			if len(sections) > 0 {
				list = strings.Join(sections, ", ")
			}
			fmt.Printf("  • %s\n", p.Name)
			fmt.Printf("      %s  %s:%d  models-max=%d\n", flag(p.Mode != "", "✓ .mode", "○ auto-port"), p.Host, p.Port, p.ModelsMax)
			fmt.Printf("      ini models: %s\n", list)
			if clash != "" {
				fmt.Printf("      %s\n", clash)
			}
			if !isFile(p.Ini) {
				issues = append(issues, "Missing ini for "+p.Name)
			}
			var missing []string
			for _, r := range sections {
				if !isFile(filepath.Join(agentsDir(), r+".md")) {
					missing = append(missing, r)
				}
			}
			if len(missing) > 0 {
				issues = append(issues, fmt.Sprintf("%s: missing agents/*.md for: %s", p.Name, strings.Join(missing, ", ")))
			} else {
				fmt.Printf("      agents: OK (%d roles)\n", len(sections))
			}
		}
	}

	fmt.Println()
	pid := managedPID(st)
	dash := func(n int) string {
		if n == 0 {
			return "—"
		}
		return fmt.Sprint(n)
	}
	active := st.Profile
	if active == "" {
		active = "—"
	}
	fmt.Printf("Active profile: %s\nManaged pid:    %s\n", active, dash(pid))
	if st.Profile != "" {
		if p, ok := getProfile(st.Profile); ok {
			up := portOpen(p.Host, p.Port)
			healthy := false
			if up {
				_, healthy = getJSON(p.BaseURL()+"/models", 3*time.Second)
			}
			fmt.Printf("Port open:      %s\nAPI /models:    %s\n", flag(up, "True", "False"), flag(healthy, "OK", "FAIL"))
			if up && !healthy {
				issues = append(issues, fmt.Sprintf("%s port is open but /v1/models failed — check ai-mode logs", st.Profile))
			}
			if pid != 0 && !up {
				issues = append(issues, fmt.Sprintf("pid %d alive but port %d closed — stale state?", pid, p.Port))
			}
			if pid == 0 && up {
				issues = append(issues, fmt.Sprintf("port %d open but not ai-mode-managed (foreign process?)", p.Port))
			}
		} else {
			issues = append(issues, fmt.Sprintf("Active profile '%s' has no matching .ini anymore", st.Profile))
		}
	}

	localBin := filepath.Join(homeDir, ".local", "bin")
	onPath := contains(filepath.SplitList(os.Getenv("PATH")), localBin)
	fmt.Printf("~/.local/bin on PATH: %s\n\n", flag(onPath, "yes", "NO"))
	if !onPath {
		issues = append(issues, "~/.local/bin is not on PATH. Run: "+pathFixCommand())
	}

	if len(issues) > 0 {
		fmt.Println("Issues:")
		for _, i := range issues {
			fmt.Printf("  ✗ %s\n", i)
		}
		return 1
	}
	fmt.Println("All checks passed.")
	return 0
}

// pathFixCommand returns a copy-pasteable command that puts ~/.local/bin on
// PATH for the user's shell and reloads it.
func pathFixCommand() string {
	switch filepath.Base(os.Getenv("SHELL")) {
	case "fish":
		return `fish_add_path "$HOME/.local/bin"`
	case "bash":
		rc := "~/.bashrc"
		if _, err := os.Stat(filepath.Join(homeDir, ".bashrc")); err != nil {
			rc = "~/.bash_profile"
		}
		return `echo 'export PATH="$HOME/.local/bin:$PATH"' >> ` + rc + " && source " + rc
	default:
		return `echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshenv && source ~/.zshenv`
	}
}
