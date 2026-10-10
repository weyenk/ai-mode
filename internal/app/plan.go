package app

// A plan is the markdown document architect writes with skills/drafting-plans.
// `ai-mode plan lint` checks its structure; `ai-mode plan verify` applies a task's
// code blocks to a scratch copy of the repo and checks red then green.

type planTask struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Kind       string   `json:"kind"`   // contract | impl | integration | docs
	Packet     string   `json:"packet"` // code | spec
	DependsOn  []string `json:"depends_on"`
	Owns       []string `json:"owns"`
	Produces   []string `json:"produces"`
	Consumes   []string `json:"consumes"`
	Covers     []string `json:"covers"`
	TestLevel  string   `json:"test_level"`
	StackLayer int      `json:"stack_layer"`
	Branch     string   `json:"branch"`
}

type planBlock struct {
	Lang, File, Mode, Anchor, Phase, Code string
}

type planPacket struct {
	ID      string
	Text    string
	Blocks  []planBlock
	RunCmd  string
	Excerpt string // the text after "**Spec excerpt:**", if any
}

type planCov struct{ Scenario, Task, Test string }

type planDoc struct {
	Title, Context string
	SpecPath       string // from the "**Spec:**" header line
	SpecText       string // the spec file's contents, loaded by the caller; empty means do not check excerpts
	Sections       map[string]bool
	Tasks          []planTask
	HotFiles       []string
	Coverage       []planCov
	Packets        map[string]*planPacket
}

type planFinding struct{ Level, Msg string } // Level: ERR | WARN

type planStep struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// planVerifyResult.Status is "verified", "failed" or "n/a" (nothing executable to check, see Note).
type planVerifyResult struct {
	Task   string     `json:"task"`
	Status string     `json:"status"`
	Note   string     `json:"note,omitempty"`
	Steps  []planStep `json:"steps"`
}
