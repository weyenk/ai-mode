package app

import (
	"os"
	"path/filepath"
	"testing"
)

const applyPlanDoc = "# P\n\n## Task graph\n\n```json\n{\"tasks\":[{\"id\":\"T01\",\"title\":\"t\",\"kind\":\"contract\",\"packet\":\"code\",\"depends_on\":[],\"owns\":[\"a.go\",\"b.go\"],\"produces\":[],\"consumes\":[],\"covers\":[],\"test_level\":\"contract\",\"stack_layer\":1,\"branch\":\"b1\"}]}\n```\n\n### T01: t\n\n**Phase A: one.**\n\n```go file=a.go\npackage x\n```\n\n**Phase B: two.**\n\n```go file=b.go\npackage x\n```\n"

func writeApplyPlan(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "plan.md")
	if err := os.WriteFile(p, []byte(applyPlanDoc), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPlanApply_WritesOnlyTheRequestedPhase(t *testing.T) {
	repo := t.TempDir()
	if code := cmdPlan([]string{"apply", writeApplyPlan(t), "--task", "T01", "--phase", "A", "--repo", repo}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if _, err := os.Stat(filepath.Join(repo, "a.go")); err != nil {
		t.Fatalf("phase A file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "b.go")); err == nil {
		t.Fatal("phase B file written by --phase A")
	}
}

func TestPlanApply_DefaultIsAllPhasesAndRejectsBadInput(t *testing.T) {
	repo := t.TempDir()
	p := writeApplyPlan(t)
	if code := cmdPlan([]string{"apply", p, "--task", "T01", "--repo", repo}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	for _, f := range []string{"a.go", "b.go"} {
		if _, err := os.Stat(filepath.Join(repo, f)); err != nil {
			t.Fatalf("%s missing: %v", f, err)
		}
	}
	if code := cmdPlan([]string{"apply", p, "--task", "T99", "--repo", repo}); code == 0 {
		t.Fatal("unknown task accepted")
	}
	if code := cmdPlan([]string{"apply", p, "--task", "T01", "--phase", "C", "--repo", repo}); code == 0 {
		t.Fatal("bad phase accepted")
	}
}
