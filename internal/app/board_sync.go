package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// boardSyncer keeps a target's Vikunja project in step with its file board. The card files in the repo stay the
// record (stages read and write them, git keeps their history); the board is where people watch the work and act:
//
//   - pull (start of every tick): a task a human creates in the init column becomes a request card; a card a human
//     moves to done (after merging its PRs) becomes done.
//   - push (after every stage, in pipeline.sync): each card is a task titled "<id> <title>" in its column's bucket,
//     with stage:/type:/context: labels, a parenttask relation to its parent, follows relations for depends_on, and
//     one comment per card note, posted by the bot that wrote the note.
//
// Anything else a human changes on the board is overwritten by the next push; corrections go through the pipeline
// commands (retry, reopen, note, answer).
type boardSyncer struct {
	pl     *pipeline
	creds  boardCreds
	owner  *vkClient
	st     boardState
	stPath string
	links  string // https://github.com/<owner>/<repo>/blob/<branch>/<module prefix>, "" when unknown
	warned map[string]bool
}

type boardState struct {
	Target  string           `json:"target"`
	Project int64            `json:"project"`
	View    int64            `json:"view"`
	Buckets map[string]int64 `json:"buckets"`
	Labels  map[string]int64 `json:"labels"`
	Shared  []string         `json:"shared,omitempty"`
}

// attachBoard turns on board sync when `ai-mode board init` has stored tokens (and AI_MODE_BOARD is not off).
func (pl *pipeline) attachBoard() error {
	if os.Getenv("AI_MODE_BOARD") == "off" {
		return nil
	}
	creds, err := loadBoardCreds()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if u := firstNonEmpty(os.Getenv("AI_MODE_BOARD_URL"), configValue("BOARD_URL")); u != "" {
		creds.URL = strings.TrimRight(u, "/")
	}
	sum := sha256.Sum256([]byte(pl.target))
	s := &boardSyncer{pl: pl, creds: creds, owner: newVK(creds.URL, creds.Token), warned: map[string]bool{},
		stPath: filepath.Join(stateDir(), "board", hex.EncodeToString(sum[:])[:12]+".json"), links: githubBlobBase(pl.target)}
	if data, err := os.ReadFile(s.stPath); err == nil {
		_ = json.Unmarshal(data, &s.st)
	}
	s.st.Target = pl.target
	pl.vk = s
	return nil
}

func (s *boardSyncer) saveState() {
	_ = os.MkdirAll(filepath.Dir(s.stPath), 0o755)
	data, _ := json.MarshalIndent(s.st, "", "  ")
	_ = os.WriteFile(s.stPath, append(data, '\n'), 0o644)
}

// client is the bot that wrote a note, or ai-mode's owner token for people and unknown names.
func (s *boardSyncer) client(by string) *vkClient {
	if b, ok := s.creds.Bots[by]; ok && b.Token != "" {
		return newVK(s.creds.URL, b.Token)
	}
	return s.owner
}

func (s *boardSyncer) warnOnce(key, format string, a ...any) {
	if !s.warned[key] {
		s.warned[key] = true
		s.pl.logf("  board: "+format, a...)
	}
}

var columnColor = map[string]string{
	"init": "8a94a6", "ideation": "6c8ebf", "building": "c9962c", "review": "d9412b", "done": "2f7d4f", "blocked": "a35a00",
}

// legacyBuckets are the buckets Vikunja gives a new kanban view; ensure renames them rather than leaving them empty.
var legacyBuckets = map[string]string{"To-Do": "init", "Doing": "building", "Done": "done"}

func (s *boardSyncer) projectTitle() string { return filepath.Base(s.pl.target) }

// ensure finds or creates the project, its kanban view, the six column buckets and the labels people pick from.
func (s *boardSyncer) ensure() error {
	if s.st.Project != 0 {
		if _, err := s.owner.project(s.st.Project); err != nil {
			if vkStatus(err) != 404 && vkStatus(err) != 403 {
				return err
			}
			s.st = boardState{Target: s.pl.target}
		}
	}
	if s.st.Project == 0 {
		ps, err := s.owner.projects()
		if err != nil {
			return err
		}
		for _, p := range ps {
			if p.Title == s.projectTitle() && !p.IsArchived {
				s.st.Project = p.ID
			}
		}
		if s.st.Project == 0 {
			p, err := s.owner.createProject(vkProject{Title: s.projectTitle(), Description: "<p>Dev pipeline board for <code>" +
				html.EscapeString(s.pl.target) + "</code>, kept by ai-mode. To ask for something, add a task to <b>init</b>; " +
				"label it <code>type:feature</code>, <code>type:bug</code> or <code>type:spike</code> and a <code>context:</code> " +
				"(default feature, hobby). Move a card to <b>done</b> once you have merged its PRs. Only a human merges.</p>"})
			if err != nil {
				return err
			}
			s.st.Project = p.ID
			s.pl.logf("  board: created project %q (%d)", p.Title, p.ID)
		}
	}
	shared := map[string]bool{}
	for _, b := range s.st.Shared {
		shared[b] = true
	}
	for _, name := range boardBots {
		if shared[name] {
			continue
		}
		if err := s.owner.shareProject(s.st.Project, name, 1); err != nil && vkStatus(err) != 409 && vkStatus(err) != 400 {
			return fmt.Errorf("share the project with %s: %w", name, err)
		}
		s.st.Shared = append(s.st.Shared, name)
	}
	if s.st.View == 0 {
		views, err := s.owner.views(s.st.Project)
		if err != nil {
			return err
		}
		for _, v := range views {
			if v.ViewKind == "kanban" {
				s.st.View = v.ID
				break
			}
		}
		if s.st.View == 0 {
			return fmt.Errorf("project %d has no kanban view", s.st.Project)
		}
	}
	if len(s.st.Buckets) < len(columnOrder) {
		if err := s.ensureBuckets(); err != nil {
			return err
		}
	}
	if s.st.Labels == nil {
		s.st.Labels = map[string]int64{}
	}
	for _, l := range []string{"type:feature", "type:bug", "type:spike", "context:hobby", "context:enterprise", "context:health"} {
		if _, err := s.label(l); err != nil {
			return err
		}
	}
	s.saveState()
	return nil
}

func (s *boardSyncer) ensureBuckets() error {
	bs, err := s.owner.buckets(s.st.Project, s.st.View)
	if err != nil {
		return err
	}
	have := map[string]vkBucket{}
	for _, b := range bs {
		have[b.Title] = b
	}
	s.st.Buckets = map[string]int64{}
	for i, col := range columnOrder {
		pos := float64((i + 1) * 100)
		if b, ok := have[col]; ok {
			s.st.Buckets[col] = b.ID
			continue
		}
		renamed := false
		for old, to := range legacyBuckets {
			if b, ok := have[old]; ok && to == col {
				b.Title, b.Position = col, pos
				if err := s.owner.updateBucket(s.st.Project, s.st.View, b); err != nil {
					return err
				}
				s.st.Buckets[col], renamed = b.ID, true
				delete(have, old)
			}
		}
		if renamed {
			continue
		}
		b, err := s.owner.createBucket(s.st.Project, s.st.View, vkBucket{Title: col, Position: pos})
		if err != nil {
			return err
		}
		s.st.Buckets[col] = b.ID
	}
	return nil
}

func (s *boardSyncer) label(name string) (int64, error) {
	if id, ok := s.st.Labels[name]; ok {
		return id, nil
	}
	ls, err := s.owner.labels()
	if err != nil {
		return 0, err
	}
	for _, l := range ls {
		if strings.Contains(l.Title, ":") {
			s.st.Labels[l.Title] = l.ID
		}
	}
	if id, ok := s.st.Labels[name]; ok {
		return id, nil
	}
	color := "8a94a6"
	if kind, val, _ := strings.Cut(name, ":"); kind == "stage" {
		if c, ok := columnColor[stageColumn[val]]; ok {
			color = c
		}
	} else if kind == "type" {
		color = "5b6b8c"
	}
	l, err := s.owner.createLabel(vkLabel{Title: name, HexColor: color})
	if err != nil {
		return 0, err
	}
	s.st.Labels[name] = l.ID
	return l.ID, nil
}

func cardDepth(t string) int {
	switch t {
	case "spec":
		return 1
	case "plan":
		return 2
	case "story":
		return 3
	}
	return 0
}

func lastBot(c *card) string {
	for i := len(c.Notes) - 1; i >= 0; i-- {
		if strings.HasPrefix(c.Notes[i].By, "bot-") {
			return c.Notes[i].By
		}
	}
	return ""
}

// push mirrors every card onto the board, parents first. Errors on one card do not stop the others.
func (s *boardSyncer) push() error {
	if err := s.ensure(); err != nil {
		return err
	}
	cards, err := s.pl.board.all()
	if err != nil {
		return err
	}
	sort.SliceStable(cards, func(i, j int) bool {
		if a, b := cardDepth(cards[i].Type), cardDepth(cards[j].Type); a != b {
			return a < b
		}
		return cards[i].ID < cards[j].ID
	})
	tasks := map[string]int64{}
	for _, c := range cards {
		tasks[c.ID] = c.BoardTask
	}
	var failed []string
	for i := range cards {
		c := &cards[i]
		if err := s.pushCard(c, cards, tasks); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", c.ID, err))
		}
		tasks[c.ID] = c.BoardTask
	}
	s.saveState()
	if len(failed) > 0 {
		return errors.New(strings.Join(failed, "; "))
	}
	return nil
}

func (s *boardSyncer) pushCard(c *card, cards []card, tasks map[string]int64) error {
	before := *c
	before.Notes = nil
	saved := func() error {
		after := *c
		after.Notes = nil
		if after.BoardTask == before.BoardTask && after.BoardNotes == before.BoardNotes {
			return nil
		}
		return s.pl.board.save(*c)
	}
	actor := s.client(lastBot(c))
	title := c.ID + " " + c.Title
	desc := s.describe(c)
	if c.BoardTask != 0 {
		if err := actor.patchTask(c.BoardTask, map[string]any{"title": title, "description": desc}); err != nil {
			if vkStatus(err) != 404 {
				return err
			}
			s.pl.logf("  board: task %d for %s is gone; recreating it", c.BoardTask, c.ID)
			c.BoardTask, c.BoardNotes = 0, 0
		}
	}
	col := columnOf(cards, c.ID)
	if c.BoardTask == 0 {
		creator := ""
		if len(c.Notes) > 0 {
			creator = c.Notes[0].By
		}
		t, err := s.client(creator).createTask(s.st.Project, vkTask{Title: title, Description: desc, BucketID: s.st.Buckets[col]})
		if err != nil {
			return err
		}
		c.BoardTask = t.ID
		if err := saved(); err != nil {
			return err
		}
	}
	if err := actor.moveTask(s.st.Project, s.st.View, s.st.Buckets[col], c.BoardTask); err != nil {
		return fmt.Errorf("move to %s: %w", col, err)
	}
	t, err := s.owner.task(c.BoardTask)
	if err != nil {
		return err
	}
	if err := s.syncLabels(c, t, actor); err != nil {
		return err
	}
	rel := func(kind, otherID string) error {
		other := tasks[otherID]
		if other == 0 {
			return nil
		}
		for _, r := range t.RelatedTasks[kind] {
			if r.ID == other {
				return nil
			}
		}
		return actor.relate(c.BoardTask, other, kind)
	}
	if c.Parent != "" {
		if err := rel("parenttask", c.Parent); err != nil {
			return fmt.Errorf("parent link: %w", err)
		}
	}
	for _, d := range c.DependsOn {
		if err := rel("follows", d); err != nil {
			return fmt.Errorf("depends-on link: %w", err)
		}
	}
	for c.BoardNotes < len(c.Notes) {
		n := c.Notes[c.BoardNotes]
		if err := s.client(n.By).comment(c.BoardTask, s.noteHTML(n)); err != nil {
			return fmt.Errorf("note %d: %w", c.BoardNotes+1, err)
		}
		c.BoardNotes++
	}
	return saved()
}

// syncLabels sets the card's stage:, type: and context: labels and removes stale ones of those kinds. A label is
// attached by the acting bot; if Vikunja refuses (a bot may only use labels it can see), the owner attaches it.
func (s *boardSyncer) syncLabels(c *card, t vkTask, actor *vkClient) error {
	want := map[string]bool{"stage:" + c.Stage: true, "type:" + c.Type: true}
	if c.Context != "" {
		want["context:"+c.Context] = true
	}
	have := map[string]int64{}
	for _, l := range t.Labels {
		have[l.Title] = l.ID
	}
	for name := range want {
		if _, ok := have[name]; ok {
			continue
		}
		id, err := s.label(name)
		if err != nil {
			return err
		}
		if err := actor.addLabel(c.BoardTask, id); err != nil {
			if actor == s.owner {
				return err
			}
			s.warnOnce("label-fallback", "a bot could not attach a label the owner created (%v); attaching labels as the owner", err)
			if err := s.owner.addLabel(c.BoardTask, id); err != nil {
				return err
			}
		}
	}
	for name, id := range have {
		kind, _, _ := strings.Cut(name, ":")
		if (kind == "stage" || kind == "type" || kind == "context") && !want[name] {
			if err := s.owner.removeLabel(c.BoardTask, id); err != nil {
				return err
			}
		}
	}
	return nil
}

// pull applies what humans did on the board since the last sync and commits any change it made.
func (s *boardSyncer) pull() error {
	if err := s.ensure(); err != nil {
		return err
	}
	buckets, err := s.owner.bucketTasks(s.st.Project, s.st.View)
	if err != nil {
		return err
	}
	colOf := map[int64]string{}
	for col, id := range s.st.Buckets {
		colOf[id] = col
	}
	cards, err := s.pl.board.all()
	if err != nil {
		return err
	}
	byTask := map[int64]*card{}
	for i := range cards {
		if cards[i].BoardTask != 0 {
			byTask[cards[i].BoardTask] = &cards[i]
		}
	}
	var changed []string
	for _, b := range buckets {
		col := colOf[b.ID]
		for _, t := range b.Tasks {
			c := byTask[t.ID]
			who := "a human"
			if t.CreatedBy != nil && t.CreatedBy.Username != "" {
				who = t.CreatedBy.Username
			}
			switch {
			case c == nil && col == "init" && !strings.HasPrefix(who, "bot-"):
				typ, ctx := "feature", "hobby"
				for _, l := range t.Labels {
					if v, ok := strings.CutPrefix(l.Title, "type:"); ok && isRequestType(v) {
						typ = v
					}
					if v, ok := strings.CutPrefix(l.Title, "context:"); ok && v != "" {
						ctx = v
					}
				}
				nc, err := s.pl.createRequest(t.Title, typ, ctx, htmlToText(t.Description), who+" (on the board)", t.ID)
				if err != nil {
					return err
				}
				changed = append(changed, nc.ID+" new from board task "+fmt.Sprint(t.ID))
			case c != nil && col == "done" && c.Stage == "review":
				c.Stage = "done"
				// Vikunja does not record who moves a task between buckets, and only people move cards to done.
				s.pl.addNote(c, cardNote{By: "human (on the board)", Stage: "review → done", Done: "Moved to done on the board.",
					Why: "A human merged the PRs.", Next: "Nothing; the parent cards follow their stories."})
				if err := s.pl.board.save(*c); err != nil {
					return err
				}
				changed = append(changed, c.ID+" done")
			}
		}
	}
	if len(changed) > 0 {
		s.pl.logf("  board: %s", strings.Join(changed, ", "))
		s.pl.sync("pipeline: from the board: " + truncateRunes(strings.Join(changed, ", "), 60))
	}
	return nil
}

func (s *boardSyncer) link(rel string) string {
	if strings.HasPrefix(rel, "http://") || strings.HasPrefix(rel, "https://") {
		return fmt.Sprintf(`<a href="%s">%s</a>`, html.EscapeString(rel), html.EscapeString(rel))
	}
	if s.links == "" {
		return "<code>" + html.EscapeString(rel) + "</code>"
	}
	u := s.links + rel
	if strings.HasSuffix(rel, "/") {
		u = strings.Replace(u, "/blob/", "/tree/", 1)
	}
	return fmt.Sprintf(`<a href="%s">%s</a>`, html.EscapeString(u), html.EscapeString(rel))
}

func (s *boardSyncer) describe(c *card) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<p><b>%s</b> · %s · stage <code>%s</code>", c.ID, html.EscapeString(c.Type), html.EscapeString(c.Stage))
	if c.Context != "" {
		fmt.Fprintf(&b, " · context %s", html.EscapeString(c.Context))
	}
	b.WriteString("</p>")
	if c.Parent != "" || len(c.DependsOn) > 0 {
		b.WriteString("<p>")
		if c.Parent != "" {
			fmt.Fprintf(&b, "Parent: %s. ", c.Parent)
		}
		if len(c.DependsOn) > 0 {
			fmt.Fprintf(&b, "Depends on: %s.", strings.Join(c.DependsOn, ", "))
		}
		b.WriteString("</p>")
	}
	if rel := c.Artifacts["request"]; rel != "" {
		if data, err := os.ReadFile(s.pl.abs(rel)); err == nil {
			if _, req, ok := strings.Cut(string(data), "## Request\n"); ok {
				fmt.Fprintf(&b, "<p><b>Request</b></p><p>%s</p>", textHTML(truncateRunes(strings.TrimSpace(req), 4000)))
			}
		}
	}
	if len(c.Artifacts) > 0 {
		kinds := make([]string, 0, len(c.Artifacts))
		for k := range c.Artifacts {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		b.WriteString("<p><b>Artifacts</b></p><ul>")
		for _, k := range kinds {
			fmt.Fprintf(&b, "<li>%s: %s</li>", html.EscapeString(k), s.link(c.Artifacts[k]))
		}
		b.WriteString("</ul>")
	}
	fmt.Fprintf(&b, "<p><i>Kept in step with <code>docs/pipeline/board/%s.json</code> by ai-mode; edits here are overwritten. "+
		"Notes arrive as comments from the bot that wrote them.</i></p>", c.ID)
	return b.String()
}

func (s *boardSyncer) noteHTML(n cardNote) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<p><b>%s</b> · %s</p>", html.EscapeString(n.Stage), textHTML(n.Done))
	if n.Why != "" {
		fmt.Fprintf(&b, "<p>%s</p>", textHTML(n.Why))
	}
	if len(n.Artifacts) > 0 {
		links := make([]string, len(n.Artifacts))
		for i, a := range n.Artifacts {
			links[i] = s.link(a)
		}
		fmt.Fprintf(&b, "<p>Artifacts: %s</p>", strings.Join(links, ", "))
	}
	if n.Next != "" {
		fmt.Fprintf(&b, "<p><i>Next:</i> %s</p>", textHTML(n.Next))
	}
	fmt.Fprintf(&b, "<p><small>%s · %s</small></p>", html.EscapeString(n.By), html.EscapeString(n.TS))
	return b.String()
}

func textHTML(s string) string {
	return strings.ReplaceAll(html.EscapeString(s), "\n", "<br>")
}

var (
	reLineEnd  = regexp.MustCompile(`(?i)<br\s*/?>|</li>`)
	reBlockEnd = regexp.MustCompile(`(?i)</(p|div|h[1-6]|ul|ol|pre|blockquote)>`)
	reTag      = regexp.MustCompile(`<[^>]*>`)
	reBlank    = regexp.MustCompile(`\n{3,}`)
)

// htmlToText turns a task description from the board's editor into the plain text a request file holds.
func htmlToText(s string) string {
	s = reLineEnd.ReplaceAllString(s, "\n")
	s = reBlockEnd.ReplaceAllString(s, "\n\n")
	s = reTag.ReplaceAllString(s, "")
	return strings.TrimSpace(reBlank.ReplaceAllString(html.UnescapeString(s), "\n\n"))
}

var reGitHubRemote = regexp.MustCompile(`github\.com[:/]([^/]+)/([^/]+?)(\.git)?$`)

// githubBlobBase links artifacts to the pipeline branch on GitHub; "" when the target is not a GitHub checkout.
func githubBlobBase(target string) string {
	remote, err := gitIn(target, "remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	m := reGitHubRemote.FindStringSubmatch(strings.TrimSpace(remote))
	if m == nil {
		return ""
	}
	branch, err := gitIn(target, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil || branch == "HEAD" {
		return ""
	}
	prefix, _ := gitIn(target, "rev-parse", "--show-prefix")
	return fmt.Sprintf("https://github.com/%s/%s/blob/%s/%s", m[1], m[2], branch, prefix)
}
