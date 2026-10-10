package app

import (
	"fmt"
	"os"
	"strings"
)

// cmdPlanApply writes one task's file blocks into a repo, so executors apply a packet exactly as written.
func cmdPlanApply(args []string) int {
	fs := newFlags("plan apply")
	taskF := fs.String("task", "", "Task id whose file blocks to write")
	phase := fs.String("phase", "all", "Which blocks: A, B or all")
	repo := fs.String("repo", ".", "Repository to write into")
	pos, code, ok := parse(fs, args)
	if !ok {
		return code
	}
	if len(pos) != 1 || *taskF == "" {
		return fail("usage: ai-mode plan apply <plan.md> --task Tnn [--phase A|B|all] [--repo DIR]")
	}
	want := strings.ToUpper(*phase)
	if want != "A" && want != "B" && want != "ALL" {
		return fail("--phase must be A, B or all, got %q", *phase)
	}
	data, err := os.ReadFile(pos[0])
	if err != nil {
		return fail("%v", err)
	}
	d, err := parsePlan(string(data))
	if err != nil {
		return fail("%v", err)
	}
	pk := d.Packets[*taskF]
	if pk == nil {
		return fail("task %s is not in the plan (or has no packet)", *taskF)
	}
	n := 0
	for _, b := range pk.Blocks {
		if b.File == "" || (want != "ALL" && b.Phase != want) {
			continue
		}
		if err := applyPlanBlock(*repo, b); err != nil {
			return fail("%s: %v", b.File, err)
		}
		fmt.Printf("wrote %s\n", b.File)
		n++
	}
	if n == 0 {
		return fail("task %s has no file blocks for phase %s", *taskF, *phase)
	}
	return 0
}
