package app

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var (
	reLogSpawn        = regexp.MustCompile(`spawning server instance with name=(\w+) on port (\d+)`)
	reLogProxy        = regexp.MustCompile(`proxying request to model (\w+) on port (\d+)`)
	reLogInstanceExit = regexp.MustCompile(`instance name=(\w+) exited`)
	reLogChildPort    = regexp.MustCompile(`^\[(\d+)\]`)
)

// logView turns raw llama-server router logs into role-annotated lines.
// The router logs child output as "[<port>] ..." and maps port→role in
// spawn/proxy lines, so we learn the mapping as lines stream by.
type logView struct {
	portToRole map[int]string
	annotate   bool
	role       string
	roleRe     *regexp.Regexp
}

func newLogView(annotate bool, role string) *logView {
	v := &logView{portToRole: map[int]string{}, annotate: annotate, role: role}
	if role != "" {
		v.roleRe = regexp.MustCompile(`\bmodel ` + regexp.QuoteMeta(role) + `\b`)
	}
	return v
}

func (v *logView) learn(line string) {
	if m := reLogSpawn.FindStringSubmatch(line); m != nil {
		port, _ := strconv.Atoi(m[2])
		v.portToRole[port] = m[1]
		return
	}
	if m := reLogProxy.FindStringSubmatch(line); m != nil {
		port, _ := strconv.Atoi(m[2])
		v.portToRole[port] = m[1]
		return
	}
	if m := reLogInstanceExit.FindStringSubmatch(line); m != nil {
		for port, r := range v.portToRole {
			if r == m[1] {
				delete(v.portToRole, port)
			}
		}
	}
}

func (v *logView) matchesRole(line string) bool {
	if strings.Contains(line, "name="+v.role) || v.roleRe.MatchString(line) {
		return true
	}
	if m := reLogChildPort.FindStringSubmatch(line); m != nil {
		port, _ := strconv.Atoi(m[1])
		return v.portToRole[port] == v.role
	}
	return false
}

// process returns the line to print, or ok=false when filtered out.
func (v *logView) process(line string) (string, bool) {
	v.learn(line)
	if v.role != "" && !v.matchesRole(line) {
		return "", false
	}
	if v.annotate {
		if m := reLogChildPort.FindStringSubmatchIndex(line); m != nil {
			port, _ := strconv.Atoi(line[m[2]:m[3]])
			if role := v.portToRole[port]; role != "" {
				return fmt.Sprintf("[%s:%d]%s", role, port, line[m[1]:]), true
			}
		}
	}
	return line, true
}

func readLines(path string, v *logView) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		if line, ok := v.process(sc.Text()); ok {
			out = append(out, line)
		}
	}
	return out, sc.Err()
}

func inode(path string) uint64 {
	if st, err := os.Stat(path); err == nil {
		if s, ok := st.Sys().(*syscall.Stat_t); ok {
			return uint64(s.Ino)
		}
	}
	return 0
}

// follow prints lines as the log grows; on truncation/rotation it relearns the
// port map and reopens.
func follow(path string, v *logView) {
	for {
		f, err := os.Open(path)
		if err != nil {
			time.Sleep(250 * time.Millisecond)
			continue
		}
		ino := inode(path)
		pos, _ := f.Seek(0, 2)
		r := bufio.NewReader(f)
		pending := ""
		rotated := false
		for !rotated {
			chunk, err := r.ReadString('\n')
			pending += chunk
			if err == nil {
				pos += int64(len(chunk))
				if line, ok := v.process(pending[:len(pending)-1]); ok {
					fmt.Println(line)
				}
				pending = ""
				continue
			}
			time.Sleep(250 * time.Millisecond)
			st, serr := os.Stat(path)
			if serr == nil && (inode(path) != ino || st.Size() < pos) {
				rotated = true
			}
		}
		f.Close()
		v.portToRole = map[int]string{}
		_, _ = readLines(path, v) // rebuild port map (output discarded)
	}
}

func cmdLogs(args []string) int {
	fs := newFlags("logs")
	followF := fs.Bool("f", false, "Follow")
	fs.BoolVar(followF, "follow", false, "Follow")
	lines := fs.Int("n", 80, "Lines of history")
	fs.IntVar(lines, "lines", 80, "Lines of history")
	noAnnotate := fs.Bool("no-annotate", false, "Raw lines without [role:port] on child server entries")
	role := fs.String("role", "", "Only lines for this role")
	pos, code, ok := parse(fs, args)
	if !ok {
		return code
	}
	name := loadState().Profile
	if len(pos) > 0 {
		name = pos[0]
	}
	if name == "" {
		return fail("No active profile.")
	}
	path := fmt.Sprintf("%s/%s.log", logDir(), name)
	if p, found := getProfile(name); found {
		path = p.LogFile()
	}
	if !isFile(path) {
		return fail("No log yet: %s", path)
	}
	v := newLogView(!*noAnnotate, *role)
	all, err := readLines(path, v)
	if err != nil {
		return fail("%v", err)
	}
	if *lines > 0 && len(all) > *lines {
		all = all[len(all)-*lines:]
	}
	for _, l := range all {
		fmt.Println(l)
	}
	if *followF {
		follow(path, v)
	}
	return 0
}
