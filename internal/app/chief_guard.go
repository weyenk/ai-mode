package app

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// The PreToolUse hook also keeps chief from doing coder's work. Prompt rules did
// not hold: on a captured session chief got a plan from architect, then called
// enter_plan_mode, rewrote it, and took Qwen Code's "User approved. You can now
// start coding." as permission. It wrote Go files with write_file/edit, ran
// `go get`, and looped on `make clean && make build`, which deleted the ai-mode
// binary the session was using.

//go:embed chief_plan_mode_denied.md
var chiefPlanModeDenied string

//go:embed chief_write_denied.md
var chiefWriteDenied string

//go:embed chief_shell_denied.md
var chiefShellDenied string

// chiefGuardedTools is the PreToolUse matcher: every tool chiefDenial checks.
const chiefGuardedTools = "^(skill|enter_plan_mode|write_file|edit|run_shell_command)$"

// chiefDenial returns why a PreToolUse payload is denied, or "" to allow it.
// Only chief is guarded. Qwen Code does not send the model, so it is read from
// the newest assistant record in the transcript; when that can't be read the
// call counts as chief's, since chief is the front door.
func chiefDenial(input []byte) string {
	var p struct {
		ToolName  string `json:"tool_name"`
		ToolInput struct {
			Skill    string `json:"skill"`
			FilePath string `json:"file_path"`
			Command  string `json:"command"`
		} `json:"tool_input"`
		TranscriptPath string `json:"transcript_path"`
		Cwd            string `json:"cwd"`
	}
	if json.Unmarshal(input, &p) != nil {
		return ""
	}
	var reason string
	switch p.ToolName {
	case "skill":
		if architectSkills[p.ToolInput.Skill] {
			reason = strings.TrimSpace(chiefSkillDenied)
		}
	case "enter_plan_mode":
		reason = strings.TrimSpace(chiefPlanModeDenied)
	case "write_file", "edit":
		if !chiefMayWrite(p.ToolInput.FilePath, hookCwd(p.Cwd)) {
			reason = fmt.Sprintf("Blocked: chief may not write %s.\n\n%s", p.ToolInput.FilePath, strings.TrimSpace(chiefWriteDenied))
		}
	case "run_shell_command":
		if seg, why := shellDenial(p.ToolInput.Command, hookCwd(p.Cwd)); why != "" {
			reason = fmt.Sprintf("Blocked: `%s` %s.\n\n%s", seg, why, strings.TrimSpace(chiefShellDenied))
		}
	}
	if p.ToolName == "run_shell_command" {
		if seg := mergeDenial(p.ToolInput.Command); seg != "" {
			// every model, not only chief: only a human merges (dev pipeline roadmap, P8c)
			return fmt.Sprintf("Blocked: `%s` merges a pull request. Only a human merges; leave the PR open and tell the human it is ready.", seg)
		}
	}
	if reason == "" {
		return ""
	}
	if model := transcriptModel(p.TranscriptPath); model != "" && model != "chief" {
		return ""
	}
	return reason
}

// mergeDenial returns the simple command that merges a PR (gh pr merge, gh stack merge), or "".
func mergeDenial(cmd string) string {
	segs, _ := shellParse(cmd)
	for _, words := range segs {
		words = realCommand(words)
		if len(words) == 0 {
			continue
		}
		switch filepath.Base(words[0]) {
		case "gh":
			args := words[1:]
			for len(args) > 0 && strings.HasPrefix(args[0], "-") {
				if (args[0] == "-R" || args[0] == "--repo") && len(args) > 1 {
					args = args[1:]
				}
				args = args[1:]
			}
			if len(args) >= 2 && (args[0] == "pr" || args[0] == "stack") && args[1] == "merge" {
				return strings.Join(words, " ")
			}
		case "bash", "sh", "zsh":
			for i, a := range words[1:] {
				if a == "-c" && i+2 < len(words) {
					if seg := mergeDenial(words[i+2]); seg != "" {
						return seg
					}
				}
			}
		}
	}
	return ""
}

// realCommand skips variable assignments and wrappers (sudo, env, ...) to reach the command that runs.
func realCommand(words []string) []string {
	for len(words) > 0 {
		w := words[0]
		if strings.Contains(w, "=") && !strings.HasPrefix(w, "-") && !strings.HasPrefix(w, "=") {
			words = words[1:]
			continue
		}
		switch filepath.Base(w) {
		case "sudo", "env", "command", "exec", "time", "nohup", "xargs":
			words = words[1:]
			continue
		}
		return words
	}
	return words
}

func hookCwd(cwd string) string {
	if cwd != "" {
		return cwd
	}
	wd, _ := os.Getwd()
	return wd
}

// chiefMayWrite reports whether chief may write path: under the project's docs/,
// Qwen Code's own directory (memory), a temp directory, or /dev/null.
func chiefMayWrite(path, cwd string) bool {
	switch path {
	case "":
		return false
	case "/dev/null", "/dev/stdout", "/dev/stderr":
		return true
	}
	if strings.ContainsAny(path, "$`") {
		return false // can't tell where it lands
	}
	path = expandHome(path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	path = filepath.Clean(path)
	for _, dir := range []string{filepath.Join(cwd, "docs"), filepath.Join(homeDir, ".qwen"), os.TempDir(), "/tmp", "/private/tmp"} {
		if dir == "" || dir == "docs" {
			continue
		}
		if rel, err := filepath.Rel(filepath.Clean(dir), path); err == nil && rel != ".." && !strings.HasPrefix(rel, "../") {
			return true
		}
	}
	return false
}

var (
	sedInPlace  = regexp.MustCompile(`^(-[a-zA-Z]*i|--in-place)`)
	perlInPlace = regexp.MustCompile(`^-[a-zA-Z]*i`)
	// gitReadOnly are the git subcommands chief may run.
	gitReadOnly = map[string]bool{"status": true, "diff": true, "log": true, "show": true, "blame": true, "ls-files": true,
		"ls-tree": true, "rev-parse": true, "grep": true, "describe": true, "shortlog": true}
	gitBranchReadOnly = map[string]bool{"-a": true, "-r": true, "-v": true, "-vv": true, "--list": true, "--show-current": true, "--all": true}
)

// shellDenial checks each simple command in a shell command line against the
// blocklist and returns the offending command and why, or "", "".
func shellDenial(cmd, cwd string) (seg, why string) {
	segs, redirects := shellParse(cmd)
	for _, target := range redirects {
		if !chiefMayWrite(target, cwd) {
			return "> " + target, "writes a file outside docs/"
		}
	}
	for _, words := range segs {
		if why := commandDenial(words, cwd); why != "" {
			return strings.Join(words, " "), why
		}
	}
	return "", ""
}

func commandDenial(words []string, cwd string) string {
	words = realCommand(words)
	if len(words) == 0 {
		return ""
	}
	args := words[1:]
	switch filepath.Base(words[0]) {
	case "rm", "rmdir", "mv", "cp", "tee", "dd", "truncate", "unlink":
		return "changes files"
	case "sed":
		for _, a := range args {
			if sedInPlace.MatchString(a) {
				return "edits files in place"
			}
		}
	case "perl":
		for _, a := range args {
			if perlInPlace.MatchString(a) {
				return "edits files in place"
			}
		}
	case "make", "gmake":
		for _, a := range args {
			if a == "clean" || a == "distclean" {
				return "deletes build output (dist/ai-mode is the ai-mode binary)"
			}
		}
	case "go":
		if len(args) > 0 && (args[0] == "get" || args[0] == "mod") {
			return "changes dependencies"
		}
	case "git":
		for len(args) > 0 && strings.HasPrefix(args[0], "-") {
			if (args[0] == "-C" || args[0] == "-c") && len(args) > 1 {
				args = args[1:]
			}
			args = args[1:]
		}
		if len(args) == 0 || gitReadOnly[args[0]] {
			return ""
		}
		if args[0] == "branch" {
			ok := true
			for _, a := range args[1:] {
				ok = ok && gitBranchReadOnly[a]
			}
			if ok {
				return ""
			}
		}
		return "changes git state"
	case "ai-mode":
		if len(args) > 0 && (args[0] == "stop" || args[0] == "restart" || args[0] == "use") {
			return "stops or restarts the server chief runs on"
		}
	case "bash", "sh", "zsh":
		for i, a := range args {
			if a == "-c" && i+1 < len(args) {
				if seg, why := shellDenial(args[i+1], cwd); why != "" {
					return why + " (in `" + seg + "`)"
				}
			}
		}
	}
	return ""
}

// shellParse splits a command line into simple commands (on unquoted ;, &, |,
// newline) of unquoted words, and collects output-redirect targets. It skips
// here-document bodies. Command substitution is not followed.
func shellParse(cmd string) (segs [][]string, redirects []string) {
	var (
		words          []string
		cur            strings.Builder
		inWord         bool
		single, double bool
		redirectNext   bool
		skipNext       bool
		heredocNext    bool
		heredocs       []string
	)
	endWord := func() {
		if !inWord {
			return
		}
		w := cur.String()
		cur.Reset()
		inWord = false
		switch {
		case redirectNext:
			redirects = append(redirects, w)
			redirectNext = false
		case heredocNext:
			heredocs = append(heredocs, w)
			heredocNext = false
		case skipNext:
			skipNext = false
		default:
			words = append(words, w)
		}
	}
	endSeg := func() {
		endWord()
		if len(words) > 0 {
			segs = append(segs, words)
		}
		words = nil
	}
	rs := []rune(cmd)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case single:
			if c == '\'' {
				single = false
			} else {
				cur.WriteRune(c)
			}
			continue
		case double:
			switch {
			case c == '"':
				double = false
			case c == '\\' && i+1 < len(rs):
				i++
				cur.WriteRune(rs[i])
			default:
				cur.WriteRune(c)
			}
			continue
		}
		switch c {
		case '\'':
			single, inWord = true, true
		case '"':
			double, inWord = true, true
		case '\\':
			if i+1 < len(rs) {
				i++
				if rs[i] != '\n' {
					cur.WriteRune(rs[i])
					inWord = true
				}
			}
		case ' ', '\t':
			endWord()
		case '\n':
			endSeg()
			// Skip here-document bodies started on the line just ended.
			for _, delim := range heredocs {
				for i+1 < len(rs) {
					end := i + 1
					for end < len(rs) && rs[end] != '\n' {
						end++
					}
					line := strings.TrimLeft(string(rs[i+1:end]), "\t")
					i = end
					if line == delim {
						break
					}
				}
			}
			heredocs = nil
		case ';', '|':
			endSeg()
		case '&':
			if i+1 < len(rs) && rs[i+1] == '>' {
				endWord()
				continue // &> file: the '>' case takes the target
			}
			endSeg()
		case '>':
			if inWord && isDigits(cur.String()) {
				cur.Reset()
				inWord = false // the fd number in 2>file
			}
			endWord()
			for i+1 < len(rs) && (rs[i+1] == '>' || rs[i+1] == '|') {
				i++
			}
			if i+1 < len(rs) && rs[i+1] == '&' {
				i++ // >&2: duplicates a descriptor, no file
				for i+1 < len(rs) && (rs[i+1] == '-' || (rs[i+1] >= '0' && rs[i+1] <= '9')) {
					i++
				}
				continue
			}
			redirectNext = true
		case '<':
			if inWord && isDigits(cur.String()) {
				cur.Reset()
				inWord = false
			}
			endWord()
			if i+1 < len(rs) && rs[i+1] == '<' {
				i++
				if i+1 < len(rs) && rs[i+1] == '<' {
					i++ // <<< here-string: next word is data
					skipNext = true
					continue
				}
				if i+1 < len(rs) && rs[i+1] == '-' {
					i++
				}
				heredocNext = true
				continue
			}
			skipNext = true // input file
		default:
			cur.WriteRune(c)
			inWord = true
		}
	}
	endSeg()
	return segs, redirects
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
