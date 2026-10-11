package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The dev pipeline (docs/superpowers/plans/2026-10-11-dev-pipeline-roadmap.md) moves a request card
// through ten stages. `ai-mode pipeline tick` is one scheduler step: take the lock, pick one card,
// run its next stage, move it, note it, commit the artifacts. Until the Vikunja board (P1) exists the
// board is one JSON file per card under <target>/docs/pipeline/board/, rendered to BOARD.md.

// cardNote is the note a stage leaves when it finishes or moves a card (roadmap "Card notes").
type cardNote struct {
	TS        string   `json:"ts"`
	By        string   `json:"by"`
	Stage     string   `json:"stage"` // "viability → spec"
	Done      string   `json:"done"`
	Artifacts []string `json:"artifacts,omitempty"`
	Why       string   `json:"why"`
	Next      string   `json:"next"`
}

type card struct {
	ID        string            `json:"id"`
	Title     string            `json:"title"`
	Type      string            `json:"type"`  // feature | bug | spike (requests); spec | plan | story
	Stage     string            `json:"stage"` // stage label; the column derives from it
	Context   string            `json:"context,omitempty"`
	Parent    string            `json:"parent,omitempty"`
	Order     int               `json:"order,omitempty"`      // position among siblings (stories: stack order)
	DependsOn []string          `json:"depends_on,omitempty"` // sibling cards that must reach review first
	Prev      string            `json:"prev_stage,omitempty"` // stage before needs-human, for retry
	Artifacts map[string]string `json:"artifacts,omitempty"`  // kind -> path relative to the target
	Created   string            `json:"created"`
	Updated   string            `json:"updated"`
	Notes     []cardNote        `json:"notes,omitempty"`
}

func (c *card) setArtifact(kind, rel string) {
	if c.Artifacts == nil {
		c.Artifacts = map[string]string{}
	}
	c.Artifacts[kind] = rel
}

var stageColumn = map[string]string{
	"new":       "init",
	"viability": "ideation", "success-metrics": "ideation", "spec": "ideation",
	"planning": "building", "amigos": "building", "ready": "building", "in-progress": "building",
	"architect-review": "building", "handoff": "building",
	"review": "review", "done": "done",
	"needs-human": "blocked", "rejected": "blocked",
}

var columnOrder = []string{"init", "ideation", "building", "review", "done", "blocked"}

func columnIndex(col string) int {
	for i, c := range columnOrder {
		if c == col {
			return i
		}
	}
	return len(columnOrder)
}

func isRequestType(t string) bool { return t == "feature" || t == "bug" || t == "spike" }

// columnOf is a card's column; a parent that is tracking its children takes the least advanced column of the
// children that are not blocked (a blocked child shows in blocked on its own).
func columnOf(cards []card, id string) string {
	var c *card
	for i := range cards {
		if cards[i].ID == id {
			c = &cards[i]
		}
	}
	if c == nil {
		return ""
	}
	if c.Stage != "tracking" {
		if col, ok := stageColumn[c.Stage]; ok {
			return col
		}
		return "building"
	}
	best := ""
	blocked := false
	for _, ch := range cards {
		if ch.Parent != id {
			continue
		}
		col := columnOf(cards, ch.ID)
		if col == "blocked" {
			blocked = true
			continue
		}
		if best == "" || columnIndex(col) < columnIndex(best) {
			best = col
		}
	}
	if best == "" && blocked {
		return "blocked"
	}
	if best == "" {
		return "building"
	}
	return best
}

// ---- file board -----------------------------------------------------------

type fileBoard struct{ dir string }

func (b fileBoard) path(id string) string { return filepath.Join(b.dir, id+".json") }

func (b fileBoard) load(id string) (card, error) {
	var c card
	data, err := os.ReadFile(b.path(id))
	if err != nil {
		return c, fmt.Errorf("card %s: %w", id, err)
	}
	return c, json.Unmarshal(data, &c)
}

func (b fileBoard) save(c card) error {
	if err := os.MkdirAll(b.dir, 0o755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(c, "", "  ")
	tmp := b.path(c.ID) + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, b.path(c.ID))
}

func (b fileBoard) all() ([]card, error) {
	ents, err := os.ReadDir(b.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []card
	for _, e := range ents {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		c, err := b.load(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (b fileBoard) nextID() (string, error) {
	cards, err := b.all()
	if err != nil {
		return "", err
	}
	n := 0
	for _, c := range cards {
		if v, err := strconv.Atoi(strings.TrimPrefix(c.ID, "C")); err == nil && v > n {
			n = v
		}
	}
	return fmt.Sprintf("C%04d", n+1), nil
}

// renderBoard is BOARD.md: one section per column, each card with its stage label and latest note.
func renderBoard(cards []card) string {
	var b strings.Builder
	b.WriteString("# Board\n\nGenerated by `ai-mode pipeline`; one JSON file per card in `board/`. Columns follow the dev pipeline roadmap.\n")
	byCol := map[string][]card{}
	for _, c := range cards {
		col := columnOf(cards, c.ID)
		byCol[col] = append(byCol[col], c)
	}
	for _, col := range columnOrder {
		fmt.Fprintf(&b, "\n## %s\n\n", col)
		if len(byCol[col]) == 0 {
			b.WriteString("_empty_\n")
			continue
		}
		b.WriteString("| Card | Type | Stage | Parent | Title | Latest note |\n| --- | --- | --- | --- | --- | --- |\n")
		for _, c := range byCol[col] {
			last := ""
			if len(c.Notes) > 0 {
				n := c.Notes[len(c.Notes)-1]
				last = fmt.Sprintf("%s (%s): %s", n.Stage, n.By, n.Done)
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n", c.ID, c.Type, c.Stage, c.Parent, mdCell(c.Title), mdCell(last))
		}
	}
	return b.String()
}

func mdCell(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(s), "\n", " "), "|", "\\|")
}

// ---- driver ---------------------------------------------------------------

// stageFunc runs one stage on c (it may change c and create child cards) and returns the next stage label and
// the note to leave. An error moves the card to needs-human with the error as the reason.
type stageFunc func(pl *pipeline, c *card) (next string, note cardNote, err error)

type pipeline struct {
	target   string // the Go module the pipeline builds (absolute)
	docs     string // <target>/docs/pipeline
	board    fileBoard
	stages   map[string]stageFunc // "<type>:<stage>"
	profile  Profile
	trace    string
	ask      func(role, user string, maxTokens int) (string, error)   // one consult; tests replace it
	newAgent func(spec agentSpec) (*agentSession, error)              // an agent-mode role; tests replace it
	amigos   func(dir, story string, rounds int) (int, string, error) // `ai-mode amigos --story`; tests replace it
	log      io.Writer
	git      bool // commit artifacts after each tick
	push     bool // push the pipeline branch after each commit
	now      func() string
}

func (pl *pipeline) logf(format string, a ...any) {
	if pl.log != nil {
		fmt.Fprintf(pl.log, format+"\n", a...)
	}
}

func (pl *pipeline) rel(abs string) string {
	r, err := filepath.Rel(pl.target, abs)
	if err != nil {
		return abs
	}
	return filepath.ToSlash(r)
}

func (pl *pipeline) abs(rel string) string { return filepath.Join(pl.target, filepath.FromSlash(rel)) }

func (pl *pipeline) newCard(c card) (card, error) {
	id, err := pl.board.nextID()
	if err != nil {
		return c, err
	}
	c.ID, c.Created, c.Updated = id, pl.now(), pl.now()
	return c, pl.board.save(c)
}

func (pl *pipeline) addNote(c *card, n cardNote) {
	n.TS = pl.now()
	if n.By == "" {
		n.By = "bot-chief"
	}
	c.Notes = append(c.Notes, n)
	c.Updated = n.TS
}

func storyDepth(t string) int {
	switch t {
	case "story":
		return 3
	case "plan":
		return 2
	case "spec":
		return 1
	}
	return 0
}

var finishedStages = map[string]bool{"review": true, "done": true}

// storyMayRun: amigos runs any time; from ready on, the stack is linear, so every earlier story (and every
// dependency) must have reached review first.
func storyMayRun(cards []card, s *card) bool {
	if s.Stage == "amigos" {
		return true
	}
	for _, o := range cards {
		if o.Type != "story" || o.Parent != s.Parent || o.ID == s.ID {
			continue
		}
		if (o.Order < s.Order || contains(s.DependsOn, o.ID)) && !finishedStages[o.Stage] {
			return false
		}
	}
	return true
}

// pick chooses the card the next tick advances: a card already in flight (deepest first, stories in stack
// order), otherwise the oldest new request, but only when no request is in flight (one feature at a time).
func (pl *pipeline) pick(cards []card) *card {
	var cand []*card
	for i := range cards {
		c := &cards[i]
		if _, ok := pl.stages[c.Type+":"+c.Stage]; !ok {
			continue
		}
		if isRequestType(c.Type) && c.Stage == "new" {
			continue
		}
		if c.Type == "story" && !storyMayRun(cards, c) {
			continue
		}
		cand = append(cand, c)
	}
	if len(cand) > 0 {
		sort.SliceStable(cand, func(i, j int) bool {
			a, b := cand[i], cand[j]
			if storyDepth(a.Type) != storyDepth(b.Type) {
				return storyDepth(a.Type) > storyDepth(b.Type)
			}
			if a.Order != b.Order {
				return a.Order < b.Order
			}
			return a.ID < b.ID
		})
		return cand[0]
	}
	for _, c := range cards {
		if !isRequestType(c.Type) || c.Stage == "new" || c.Stage == "rejected" {
			continue
		}
		switch columnOf(cards, c.ID) {
		case "ideation", "building", "blocked":
			return nil // a request is still in flight (or waiting on a human)
		}
	}
	for i := range cards {
		c := &cards[i]
		if isRequestType(c.Type) && c.Stage == "new" {
			if _, ok := pl.stages[c.Type+":new"]; ok {
				return c
			}
		}
	}
	return nil
}

// tick runs one stage on one card. It returns the card it moved (nil when nothing could advance).
func (pl *pipeline) tick() (*card, error) {
	cards, err := pl.board.all()
	if err != nil {
		return nil, err
	}
	c := pl.pick(cards)
	if c == nil {
		return nil, nil
	}
	from := c.Stage
	pl.logf("▶ %s %s [%s] %s", c.ID, c.Type, from, c.Title)
	start := time.Now()
	next, note, serr := pl.stages[c.Type+":"+from](pl, c)
	if serr != nil {
		next = "needs-human"
		note = cardNote{By: "bot-chief", Done: "Stage " + from + " stopped before finishing.", Why: serr.Error(),
			Next: fmt.Sprintf("A human decides how to proceed. After fixing the cause, run `ai-mode pipeline retry %s`.", c.ID)}
	}
	if next == "needs-human" {
		c.Prev = from
	}
	note.Stage = from + " → " + next
	c.Stage = next
	pl.addNote(c, note)
	if err := pl.board.save(*c); err != nil {
		return c, err
	}
	pl.logf("  %s %s in %s: %s", c.ID, note.Stage, time.Since(start).Round(time.Second), firstLine(note.Done))
	pl.sync(fmt.Sprintf("pipeline: %s %s (%s)", c.ID, note.Stage, truncateRunes(c.Title, 50)))
	return c, serr
}

// sync re-renders BOARD.md, then commits docs/pipeline (and pushes when asked). Git failures are reported, not fatal.
func (pl *pipeline) sync(msg string) {
	cards, _ := pl.board.all()
	_ = os.MkdirAll(pl.docs, 0o755)
	_ = os.WriteFile(filepath.Join(pl.docs, "BOARD.md"), []byte(renderBoard(cards)), 0o644)
	if !pl.git {
		return
	}
	rel := pl.rel(pl.docs)
	if out, err := gitIn(pl.target, "add", "-A", "--", rel); err != nil {
		pl.logf("  git add: %v %s", err, out)
		return
	}
	if out, err := gitIn(pl.target, "commit", "-q", "-m", msg+"\n\nCo-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>", "--", rel); err != nil {
		if !strings.Contains(out, "nothing") {
			pl.logf("  git commit: %v %s", err, out)
		}
		return
	}
	if pl.push {
		if out, err := gitIn(pl.target, "push", "-q"); err != nil {
			pl.logf("  git push: %v %s", err, out)
		}
	}
}

func gitIn(dir string, args ...string) (string, error) {
	c := exec.Command("git", args...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func (pl *pipeline) retry(id string) error {
	c, err := pl.board.load(id)
	if err != nil {
		return err
	}
	if c.Stage != "needs-human" || c.Prev == "" {
		return fmt.Errorf("%s is at %s, not needs-human with a previous stage", id, c.Stage)
	}
	from := c.Stage
	c.Stage, c.Prev = c.Prev, ""
	pl.addNote(&c, cardNote{By: "human", Stage: from + " → " + c.Stage, Done: "Retry requested.", Why: "A human cleared the block.", Next: "The next tick re-runs " + c.Stage + "."})
	if err := pl.board.save(c); err != nil {
		return err
	}
	pl.sync(fmt.Sprintf("pipeline: %s retry at %s", id, c.Stage))
	return nil
}

// ---- lock -------------------------------------------------------------------

func pipelineLockPath(target string) string {
	h := sha256.Sum256([]byte(target))
	return filepath.Join(stateDir(), "pipeline", "lock-"+hex.EncodeToString(h[:6]))
}

// acquireLock takes the per-target lock; a lock held by a dead process is taken over.
func acquireLock(path string) (func(), error) {
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintf(f, "%d\n", os.Getpid())
			f.Close()
			return func() { os.Remove(path) }, nil
		}
		data, _ := os.ReadFile(path)
		pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
		if pid > 0 && pidAlive(pid) {
			return nil, fmt.Errorf("work is already in flight (pid %d holds %s)", pid, path)
		}
		os.Remove(path)
	}
	return nil, fmt.Errorf("could not take %s", path)
}

// ---- command ----------------------------------------------------------------

func newPipeline(repo string, git, push bool) (*pipeline, error) {
	target, err := filepath.Abs(repo)
	if err != nil {
		return nil, err
	}
	if st, err := os.Stat(target); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("target %s is not a directory", target)
	}
	docs := filepath.Join(target, "docs", "pipeline")
	pl := &pipeline{target: target, docs: docs, board: fileBoard{dir: filepath.Join(docs, "board")}, stages: defaultStages(),
		log: os.Stderr, git: git, push: push, trace: newTraceID(),
		now: func() string { return time.Now().UTC().Format(time.RFC3339) }}
	pl.ask = func(role, user string, maxTokens int) (string, error) {
		if role == "architect" && maxTokens < architectMinTokens {
			maxTokens = architectMinTokens
		}
		var notice string
		text, code := runAsk(askParams{profile: pl.profile, role: role, user: user, maxTokens: maxTokens, timeout: 1200,
			trace: pl.trace, caller: "pipeline", onTruncated: func(n string) { notice = n }})
		if code != 0 {
			return "", fmt.Errorf("%s did not answer (see ai-mode trace %s)", role, pl.trace)
		}
		if notice != "" {
			pl.logf("  %s", notice)
		}
		return text, nil
	}
	pl.newAgent = func(s agentSpec) (*agentSession, error) { return newRoleAgent(pl.profile, s, pl.trace, "pipeline") }
	pl.amigos = amigosExec
	return pl, nil
}

// runAgentChecked runs role as an agent on task in the target, then check; while check reports findings the same
// conversation continues with them (twice at most). Findings left after that are an error.
func (pl *pipeline) runAgentChecked(role, task string, owns []string, cmds [][]string, maxTurns int, check func() []string) error {
	return pl.runAgentCheckedSystem(role, "", task, owns, cmds, maxTurns, check)
}

// runAgentCheckedSystem is runAgentChecked with a task-specific system prompt instead of the role's guide.
func (pl *pipeline) runAgentCheckedSystem(role, system, task string, owns []string, cmds [][]string, maxTurns int, check func() []string) error {
	_, err := pl.runAgentIn(pl.target, role, system, task, owns, nil, cmds, maxTurns, check)
	return err
}

// runAgentIn is the general form: any root, read-only patterns, and the last result.
func (pl *pipeline) runAgentIn(root, role, system, task string, owns, readOnly []string, cmds [][]string, maxTurns int, check func() []string) (agentResult, error) {
	spec := agentSpec{Role: role, Root: root, Owns: owns, ReadOnly: readOnly, Commands: cmds, MaxTurns: maxTurns, MaxWall: 45 * time.Minute, Log: pl.log}
	if system != "" {
		spec.System = system + agentRulesFor(owns, cmds)
	}
	a, err := pl.newAgent(spec)
	if err != nil {
		return agentResult{}, err
	}
	pl.logf("  agent %s: %s", role, firstLine(task))
	res, err := a.Run(task)
	for attempt := 0; ; attempt++ {
		if err != nil {
			return res, err
		}
		f := check()
		if len(f) == 0 {
			pl.logf("  agent %s %s after %d turns: %s", role, res.Stopped, res.Turns, firstLine(res.Summary))
			return res, nil
		}
		if attempt == 2 {
			return res, fmt.Errorf("%s stopped (%s) and the check still fails: %s", role, res.Stopped, strings.Join(f, "; "))
		}
		pl.logf("  agent %s: check found %d problem(s), continuing: %s", role, len(f), firstLine(f[0]))
		res, err = a.Continue("A check of your work found problems. Fix them, check again, then call finish:\n- " + strings.Join(f, "\n- "))
	}
}

const pipelineUsage = `usage: ai-mode pipeline <command> [--repo DIR]
  new <title> [--type feature|bug|spike] [--context hobby|enterprise|health|...] [--body FILE|-]   stage 1: create a request card
  status                 the board by column (also: board)
  show <card>            one card with its notes and artifacts
  tick [--push]          stage 2: run one stage on one card, commit the artifacts
  run [--push] [--max-ticks N]   tick until nothing can advance
  retry <card>           move a needs-human card back to the stage it stopped at
  answer <card> <text>   add the requester's answer to a story amigos paused on, and re-run amigos
  launchd [--interval S] print a LaunchAgent plist that runs tick (not installed)`

func cmdPipeline(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, pipelineUsage)
		return 2
	}
	sub := args[0]
	fs := newFlags("pipeline " + sub)
	repo := fs.String("repo", ".", "Target Go module the pipeline works on")
	push := fs.Bool("push", false, "Push the branch after each committed stage")
	noGit := fs.Bool("no-commit", false, "Do not commit artifacts")
	maxTicks := fs.Int("max-ticks", 50, "run: stop after this many ticks")
	typ := fs.String("type", "feature", "new: feature | bug | spike")
	ctxF := fs.String("context", "hobby", "new: the request's context (hobby, enterprise, health, ...)")
	body := fs.String("body", "", "new: request text from a file, or - for stdin")
	interval := fs.Int("interval", 900, "launchd: seconds between ticks")
	pos, code, ok := parse(fs, args[1:])
	if !ok {
		return code
	}
	pl, err := newPipeline(*repo, !*noGit, *push)
	if err != nil {
		return fail("%v", err)
	}
	switch sub {
	case "new":
		return pipelineNew(pl, strings.Join(pos, " "), *typ, *ctxF, *body)
	case "status", "board":
		cards, err := pl.board.all()
		if err != nil {
			return fail("%v", err)
		}
		fmt.Print(renderBoard(cards))
		return 0
	case "show":
		if len(pos) != 1 {
			return fail("usage: ai-mode pipeline show <card>")
		}
		c, err := pl.board.load(pos[0])
		if err != nil {
			return fail("%v", err)
		}
		b, _ := json.MarshalIndent(c, "", "  ")
		fmt.Println(string(b))
		return 0
	case "answer":
		if len(pos) < 2 {
			return fail("usage: ai-mode pipeline answer <card> <answer...>")
		}
		if err := pl.answer(pos[0], strings.Join(pos[1:], " ")); err != nil {
			return fail("%v", err)
		}
		return 0
	case "retry":
		if len(pos) != 1 {
			return fail("usage: ai-mode pipeline retry <card>")
		}
		if err := pl.retry(pos[0]); err != nil {
			return fail("%v", err)
		}
		return 0
	case "tick", "run":
		release, err := acquireLock(pipelineLockPath(pl.target))
		if err != nil {
			fmt.Fprintln(os.Stderr, "skip: "+err.Error())
			return 0
		}
		defer release()
		if code := pl.ensureProfile(); code != 0 {
			return code
		}
		n := 1
		if sub == "run" {
			n = *maxTicks
		}
		moved := 0
		for i := 0; i < n; i++ {
			c, err := pl.tick()
			if c == nil && err == nil {
				break
			}
			moved++
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s blocked: %v\n", c.ID, err)
			}
		}
		if moved == 0 {
			fmt.Fprintln(os.Stderr, "nothing to advance")
		}
		return 0
	case "launchd":
		self, _ := os.Executable()
		fmt.Print(launchdPlist(self, pl.target, *interval))
		return 0
	}
	fmt.Fprintln(os.Stderr, pipelineUsage)
	return 2
}

// ensureProfile makes sure dev-shop (or the active profile) is up before a stage calls a model.
func (pl *pipeline) ensureProfile() int {
	p, code := activeProfile("", "Start the studio first: ai-mode use dev-shop")
	if code != 0 {
		return code
	}
	if _, ok := getJSON(p.BaseURL()+"/models", 10*time.Second); !ok {
		return fail("API not reachable at %s; start it with: ai-mode use %s", p.BaseURL(), p.Name)
	}
	pl.profile = p
	return 0
}

func pipelineNew(pl *pipeline, title, typ, ctx, bodyPath string) int {
	title = strings.TrimSpace(title)
	if title == "" || !isRequestType(typ) {
		return fail("usage: ai-mode pipeline new <title> [--type feature|bug|spike] [--context C] [--body FILE|-]")
	}
	body := ""
	switch bodyPath {
	case "":
	case "-":
		b, _ := io.ReadAll(os.Stdin)
		body = string(b)
	default:
		b, err := os.ReadFile(bodyPath)
		if err != nil {
			return fail("%v", err)
		}
		body = string(b)
	}
	if strings.TrimSpace(body) == "" {
		body = title
	}
	c, err := pl.newCard(card{Title: title, Type: typ, Stage: "new", Context: ctx})
	if err != nil {
		return fail("%v", err)
	}
	rel := fmt.Sprintf("docs/pipeline/requests/%s-%s.md", c.ID, slugify(title))
	doc := fmt.Sprintf("# %s\n\n- **Card:** %s\n- **Type:** %s\n- **Context:** %s\n- **Requested:** %s\n\n## Request\n\n%s\n",
		title, c.ID, typ, ctx, pl.now()[:10], strings.TrimSpace(body))
	if err := os.MkdirAll(filepath.Dir(pl.abs(rel)), 0o755); err != nil {
		return fail("%v", err)
	}
	if err := os.WriteFile(pl.abs(rel), []byte(doc), 0o644); err != nil {
		return fail("%v", err)
	}
	c.setArtifact("request", rel)
	pl.addNote(&c, cardNote{By: "human", Stage: "→ new", Done: "Request created.", Artifacts: []string{rel},
		Why: "Intake.", Next: "The next tick runs viability (research, product, architect)."})
	if err := pl.board.save(c); err != nil {
		return fail("%v", err)
	}
	pl.sync(fmt.Sprintf("pipeline: %s new %s request (%s)", c.ID, typ, truncateRunes(title, 50)))
	fmt.Println(c.ID)
	return 0
}

func launchdPlist(self, target string, interval int) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.ai-mode.pipeline</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string><string>pipeline</string><string>tick</string><string>--push</string><string>--repo</string><string>%s</string>
  </array>
  <key>WorkingDirectory</key><string>%s</string>
  <key>StartInterval</key><integer>%d</integer>
  <key>StandardOutPath</key><string>%s</string>
  <key>StandardErrorPath</key><string>%s</string>
</dict>
</plist>
`, self, target, target, interval, filepath.Join(stateDir(), "pipeline", "launchd.log"), filepath.Join(stateDir(), "pipeline", "launchd.log"))
}

// defaultStages maps "<card type>:<stage label>" to the stage that runs there. Requests of every type follow the
// feature path until P3/P4 decide what bugs and spikes skip.
func defaultStages() map[string]stageFunc {
	m := map[string]stageFunc{}
	for _, t := range []string{"feature", "bug", "spike"} {
		m[t+":new"] = stageViability
		m[t+":spec"] = stageSpecSplit
	}
	m["spec:spec"] = stageSpecWrite
	m["spec:planning"] = stageFeaturePlan
	m["plan:planning"] = stageStories
	m["story:amigos"] = stageAmigos
	for k, f := range laterStages {
		m[k] = f
	}
	return m
}
