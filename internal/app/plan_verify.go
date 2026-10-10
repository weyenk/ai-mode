package app

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// verifyPlanTask checks that a code packet really is red then green. It works on a temporary copy of
// repo (the real repo is never touched), applies the task's ancestors' code first, then Phase A, then
// Phase B, running the repo's Go gates and capturing each command's own exit status.
func verifyPlanTask(d *planDoc, taskID, repo string) planVerifyResult {
	res := planVerifyResult{Task: taskID, Status: "failed"}
	step := func(name string, ok bool, detail string) bool {
		res.Steps = append(res.Steps, planStep{Name: name, OK: ok, Detail: detail})
		return ok
	}
	var task planTask
	found := false
	for _, t := range d.Tasks {
		if t.ID == taskID {
			task, found = t, true
		}
	}
	pk := d.Packets[taskID]
	if !found || pk == nil {
		step("task", false, fmt.Sprintf("task %s or its packet not found in the plan", taskID))
		return res
	}
	if task.Packet == "spec" {
		res.Status = "n/a"
		res.Note = "spec packet: no code to verify; the executors write the code and `plan tdd-check` gates it"
		return res
	}

	byID := map[string]planTask{}
	for _, t := range d.Tasks {
		byID[t.ID] = t
	}
	waves, err := planWaves(d.Tasks)
	if err != nil {
		step("graph", false, err.Error())
		return res
	}
	anc := ancestors(d.Tasks)[taskID]
	var order []string
	for id := range anc {
		order = append(order, id)
	}
	sort.Slice(order, func(i, j int) bool {
		if waves[order[i]] != waves[order[j]] {
			return waves[order[i]] < waves[order[j]]
		}
		return order[i] < order[j]
	})
	var specAnc []string
	for _, id := range order {
		if byID[id].Packet == "spec" {
			specAnc = append(specAnc, id)
		}
	}
	if len(specAnc) > 0 {
		res.Status = "n/a"
		res.Note = fmt.Sprintf("not verifiable yet: depends on spec packets %v whose code the executors write", specAnc)
		return res
	}

	work, err := os.MkdirTemp("", "ai-mode-plan-verify-")
	if err != nil {
		step("setup", false, err.Error())
		return res
	}
	defer os.RemoveAll(work)
	if err := copyRepo(repo, work); err != nil {
		step("setup", false, "copying the repo: "+err.Error())
		return res
	}

	for _, id := range order {
		for _, b := range d.Packets[id].Blocks {
			if b.File == "" {
				continue
			}
			if err := applyPlanBlock(work, b); err != nil {
				step("dependency "+id, false, err.Error())
				return res
			}
		}
	}
	apply := func(phase string) (goFiles []string, ok bool) {
		for _, b := range pk.Blocks {
			if b.File == "" || b.Phase != phase {
				continue
			}
			if err := applyPlanBlock(work, b); err != nil {
				step("phase "+phase+": apply "+b.File, false, err.Error())
				return nil, false
			}
			if strings.HasSuffix(b.File, ".go") {
				goFiles = append(goFiles, b.File)
			}
		}
		return goFiles, true
	}

	if task.Kind == "contract" {
		return verifyContract(&res, step, pk, work, apply)
	}
	if _, ok := apply("A"); !ok {
		return res
	}
	if out, code := runShell(work, "go vet ./..."); !step("phase A: compile (go vet ./...)", code == 0, tail(out)) {
		return res
	}
	out, code := runShell(work, pk.RunCmd)
	switch {
	case strings.Contains(out, "no tests to run"):
		step("phase A: red ("+pk.RunCmd+")", false, "the run pattern matched no tests\n"+tail(out))
	case code == 0:
		step("phase A: red ("+pk.RunCmd+")", false, "the tests pass against the stubs, so they prove nothing; they must fail on an assertion\n"+tail(out))
	case strings.Contains(out, "[build failed]") || strings.Contains(out, "[setup failed]"):
		step("phase A: red ("+pk.RunCmd+")", false, "the failure is a build error, not an assertion; add compiling stubs\n"+tail(out))
	default:
		step("phase A: red ("+pk.RunCmd+")", true, "fails as intended")
	}
	if !res.Steps[len(res.Steps)-1].OK {
		return res
	}

	goFiles, ok := apply("B")
	if !ok {
		return res
	}
	allGo := append([]string{}, goFiles...)
	for _, b := range pk.Blocks {
		if b.File != "" && b.Phase == "A" && strings.HasSuffix(b.File, ".go") {
			allGo = append(allGo, b.File)
		}
	}
	if len(allGo) > 0 {
		out, _ := runShell(work, "gofmt -l "+strings.Join(allGo, " "))
		if !step("phase B: gofmt", strings.TrimSpace(out) == "", "unformatted: "+strings.TrimSpace(out)) {
			return res
		}
	}
	if out, code := runShell(work, "go build ./..."); !step("phase B: build (go build ./...)", code == 0, tail(out)) {
		return res
	}
	if out, code := runShell(work, "go vet ./..."); !step("phase B: go vet ./...", code == 0, tail(out)) {
		return res
	}
	out, code = runShell(work, pk.RunCmd)
	green := code == 0 && !strings.Contains(out, "no tests to run")
	if !step("phase B: green ("+pk.RunCmd+")", green, tail(out)) {
		return res
	}
	if out, code := runShell(work, "go test ./..."); !step("phase B: full gate (go test ./...)", code == 0, tail(out)) {
		return res
	}
	res.Status = "verified"
	return res
}

// verifyContract checks a contract task: types, interfaces and fakes have no behavior to go red on, so it
// must compile, be gofmt-clean, vet clean, pass its own Run command if it has one, and keep the full gate green.
func verifyContract(res *planVerifyResult, step func(string, bool, string) bool, pk *planPacket, work string, apply func(string) ([]string, bool)) planVerifyResult {
	a, ok := apply("A")
	if !ok {
		return *res
	}
	bb, ok := apply("B")
	if !ok {
		return *res
	}
	if files := append(a, bb...); len(files) > 0 {
		out, _ := runShell(work, "gofmt -l "+strings.Join(files, " "))
		if !step("contract: gofmt", strings.TrimSpace(out) == "", "unformatted: "+strings.TrimSpace(out)) {
			return *res
		}
	}
	if out, code := runShell(work, "go build ./..."); !step("contract: build (go build ./...)", code == 0, tail(out)) {
		return *res
	}
	if out, code := runShell(work, "go vet ./..."); !step("contract: compile (go vet ./...)", code == 0, tail(out)) {
		return *res
	}
	if pk.RunCmd != "" {
		out, code := runShell(work, pk.RunCmd)
		if !step("contract: "+pk.RunCmd, code == 0, tail(out)) {
			return *res
		}
	}
	if out, code := runShell(work, "go test ./..."); !step("contract: full gate (go test ./...)", code == 0, tail(out)) {
		return *res
	}
	res.Status = "verified"
	return *res
}

// runShell runs cmd in dir and returns its combined output and its own exit status.
func runShell(dir, cmd string) (string, int) {
	c := exec.Command("sh", "-c", cmd)
	c.Dir = dir
	c.Env = append(os.Environ(), "GOWORK=off", "AI_MODE_PLAN_VERIFYING=1")
	out, err := c.CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return string(out), ee.ExitCode()
	}
	return string(out) + err.Error(), 1
}

func tail(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 1500 {
		return "…" + s[len(s)-1500:]
	}
	return s
}

// copyRepo copies src to dst, skipping VCS metadata, build output and symlinks.
func copyRepo(src, dst string) error {
	return filepath.WalkDir(src, func(p string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if de.IsDir() {
			switch de.Name() {
			case ".git", "dist", "node_modules":
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if !de.Type().IsRegular() {
			return nil
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(filepath.Join(dst, rel))
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
}

func cmdPlanVerify(args []string) int {
	fs := newFlags("plan verify")
	taskF := fs.String("task", "", "Task id to verify, or \"all\" for every task in wave order")
	repo := fs.String("repo", ".", "Repository to copy and run the gates in")
	asJSON := fs.Bool("json", false, "Print results as JSON")
	pos, code, ok := parse(fs, args)
	if !ok {
		return code
	}
	if len(pos) != 1 || *taskF == "" {
		return fail("usage: ai-mode plan verify <plan.md> --task Tnn|all [--repo DIR] [--json]")
	}
	data, err := os.ReadFile(pos[0])
	if err != nil {
		return fail("%v", err)
	}
	d, err := parsePlan(string(data))
	if err != nil {
		return fail("%v", err)
	}
	var ids []string
	if *taskF == "all" {
		w, err := planWaves(d.Tasks)
		if err != nil {
			return fail("%v", err)
		}
		for _, t := range d.Tasks {
			ids = append(ids, t.ID)
		}
		sort.SliceStable(ids, func(i, j int) bool { return w[ids[i]] < w[ids[j]] })
	} else {
		found := false
		for _, t := range d.Tasks {
			found = found || t.ID == *taskF
		}
		if !found || d.Packets[*taskF] == nil {
			return fail("task %s is not in the plan (or has no packet)", *taskF)
		}
		ids = []string{*taskF}
	}
	var results []planVerifyResult
	failed := false
	for _, id := range ids {
		r := verifyPlanTask(d, id, *repo)
		results = append(results, r)
		failed = failed || r.Status == "failed"
	}
	if *asJSON {
		b, _ := json.MarshalIndent(results, "", "  ")
		fmt.Println(string(b))
	} else {
		for _, r := range results {
			fmt.Printf("%s: %s", r.Task, r.Status)
			if r.Note != "" {
				fmt.Printf(" (%s)", r.Note)
			}
			fmt.Println()
			for _, s := range r.Steps {
				mark := "ok  "
				if !s.OK {
					mark = "FAIL"
				}
				fmt.Printf("  %s %s\n", mark, s.Name)
				if !s.OK && s.Detail != "" {
					fmt.Println("      " + strings.ReplaceAll(s.Detail, "\n", "\n      "))
				}
			}
		}
	}
	if failed {
		return 2
	}
	return 0
}
