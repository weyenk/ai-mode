package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// fakeVikunja is the slice of Vikunja's /api/v2 the board sync uses, in memory. Tokens name their user.
type fakeVikunja struct {
	mu       sync.Mutex
	next     int64
	projects map[int64]*vkProject
	shares   map[int64][]string
	buckets  map[int64][]*vkBucket // view -> buckets
	tasks    map[int64]*fakeTask
	labels   map[int64]*vkLabel
	tokens   map[string]string // bearer -> username
}

type fakeTask struct {
	vkTask
	bucket   int64
	creator  string
	comments []fakeComment
	rels     map[string][]int64
}

type fakeComment struct{ author, html string }

func newFakeVikunja(t *testing.T) (*fakeVikunja, *httptest.Server) {
	f := &fakeVikunja{next: 100, projects: map[int64]*vkProject{}, shares: map[int64][]string{}, buckets: map[int64][]*vkBucket{},
		tasks: map[int64]*fakeTask{}, labels: map[int64]*vkLabel{}, tokens: map[string]string{"owner-tok": "noah"}}
	for _, b := range boardBots {
		f.tokens["tok-"+b] = b
	}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeVikunja) id() int64 { f.next++; return f.next }

func (f *fakeVikunja) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := strings.TrimPrefix(r.URL.Path, "/api/v2")
	if path == "/info" {
		json.NewEncoder(w).Encode(map[string]any{"version": "v2.7.0"})
		return
	}
	who, ok := f.tokens[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
	if !ok {
		http.Error(w, `{"detail":"unauthorized"}`, 401)
		return
	}
	var body map[string]any
	if r.Body != nil {
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
	}
	p := strings.Split(strings.Trim(path, "/"), "/")
	n := func(i int) int64 { v, _ := strconv.ParseInt(p[i], 10, 64); return v }
	out := func(v any) { json.NewEncoder(w).Encode(v) }
	list := func(items any) { out(map[string]any{"items": items, "total_pages": 1}) }
	str := func(k string) string { s, _ := body[k].(string); return s }
	num := func(k string) int64 { v, _ := body[k].(float64); return int64(v) }
	m := r.Method
	switch {
	case path == "/token/test":
		out(map[string]string{"message": "ok"})
	case path == "/projects" && m == "GET":
		var ps []vkProject
		for _, pr := range f.projects {
			ps = append(ps, *pr)
		}
		list(ps)
	case path == "/projects" && m == "POST":
		pr := &vkProject{ID: f.id(), Title: str("title"), Description: str("description")}
		view := vkView{ID: f.id(), Title: "Kanban", ViewKind: "kanban"}
		pr.Views = []vkView{{ID: f.id(), Title: "List", ViewKind: "list"}, view}
		for i, t := range []string{"To-Do", "Doing", "Done"} {
			f.buckets[view.ID] = append(f.buckets[view.ID], &vkBucket{ID: f.id(), Title: t, Position: float64(i)})
		}
		f.projects[pr.ID] = pr
		out(pr)
	case len(p) == 2 && p[0] == "projects" && m == "GET":
		pr, ok := f.projects[n(1)]
		if !ok {
			http.Error(w, `{"detail":"not found"}`, 404)
			return
		}
		out(pr)
	case len(p) == 3 && p[2] == "users" && m == "POST":
		f.shares[n(1)] = append(f.shares[n(1)], str("username"))
		out(body)
	case len(p) == 3 && p[2] == "views":
		list(f.projects[n(1)].Views)
	case len(p) == 5 && p[4] == "buckets" && m == "GET":
		var bs []vkBucket
		for _, b := range f.buckets[n(3)] {
			bs = append(bs, *b)
		}
		list(bs)
	case len(p) == 5 && p[4] == "buckets" && m == "POST":
		b := &vkBucket{ID: f.id(), Title: str("title")}
		b.Position, _ = body["position"].(float64)
		f.buckets[n(3)] = append(f.buckets[n(3)], b)
		out(b)
	case len(p) == 6 && p[4] == "buckets" && p[5] == "tasks" && m == "GET":
		var bs []vkBucket
		for _, b := range f.buckets[n(3)] {
			bb := *b
			for _, t := range f.sortedTasks() {
				if t.bucket == b.ID {
					tt := t.vkTask
					tt.CreatedBy = &vkUser{Username: t.creator}
					bb.Tasks = append(bb.Tasks, tt)
				}
			}
			bs = append(bs, bb)
		}
		out(map[string]any{"items": bs})
	case len(p) == 6 && p[4] == "buckets" && m == "PUT":
		for _, b := range f.buckets[n(3)] {
			if b.ID == n(5) {
				b.Title = str("title")
				b.Position, _ = body["position"].(float64)
			}
		}
		out(body)
	case len(p) == 7 && p[6] == "tasks" && m == "PUT":
		f.tasks[num("task_id")].bucket = n(5)
		out(body)
	case len(p) == 3 && p[2] == "tasks" && m == "POST":
		t := &fakeTask{vkTask: vkTask{ID: f.id(), Title: str("title"), Description: str("description"), ProjectID: n(1)},
			bucket: num("bucket_id"), creator: who, rels: map[string][]int64{}}
		f.tasks[t.ID] = t
		out(t.vkTask)
	case len(p) == 2 && p[0] == "tasks" && m == "GET":
		t := f.tasks[n(1)]
		vt := t.vkTask
		vt.RelatedTasks = map[string][]vkTask{}
		for kind, ids := range t.rels {
			for _, id := range ids {
				vt.RelatedTasks[kind] = append(vt.RelatedTasks[kind], vkTask{ID: id})
			}
		}
		out(vt)
	case len(p) == 2 && p[0] == "tasks" && m == "PATCH":
		t, ok := f.tasks[n(1)]
		if !ok {
			http.Error(w, `{"detail":"not found"}`, 404)
			return
		}
		if r.Header.Get("Content-Type") != "application/merge-patch+json" {
			http.Error(w, `{"detail":"want a merge patch"}`, 415)
			return
		}
		t.Title, t.Description = str("title"), str("description")
		out(t.vkTask)
	case len(p) == 3 && p[2] == "labels" && m == "POST":
		t := f.tasks[n(1)]
		t.Labels = append(t.Labels, *f.labels[num("label_id")])
		out(body)
	case len(p) == 4 && p[2] == "labels" && m == "DELETE":
		t := f.tasks[n(1)]
		var keep []vkLabel
		for _, l := range t.Labels {
			if l.ID != n(3) {
				keep = append(keep, l)
			}
		}
		t.Labels = keep
		out(map[string]any{})
	case path == "/labels" && m == "GET":
		var ls []vkLabel
		for _, l := range f.labels {
			ls = append(ls, *l)
		}
		list(ls)
	case path == "/labels" && m == "POST":
		l := &vkLabel{ID: f.id(), Title: str("title"), HexColor: str("hex_color")}
		f.labels[l.ID] = l
		out(l)
	case len(p) == 3 && p[2] == "comments" && m == "POST":
		t := f.tasks[n(1)]
		t.comments = append(t.comments, fakeComment{author: who, html: str("comment")})
		out(body)
	case len(p) == 3 && p[2] == "relations" && m == "POST":
		t := f.tasks[n(1)]
		kind := str("relation_kind")
		t.rels[kind] = append(t.rels[kind], num("other_task_id"))
		out(body)
	default:
		http.Error(w, fmt.Sprintf(`{"detail":"fake: no route %s %s"}`, m, path), 404)
	}
}

func (f *fakeVikunja) sortedTasks() []*fakeTask {
	var ts []*fakeTask
	for _, t := range f.tasks {
		ts = append(ts, t)
	}
	sort.Slice(ts, func(i, j int) bool { return ts[i].ID < ts[j].ID })
	return ts
}

func (f *fakeVikunja) bucketTitle(id int64) string {
	for _, bs := range f.buckets {
		for _, b := range bs {
			if b.ID == id {
				return b.Title
			}
		}
	}
	return ""
}

func (f *fakeVikunja) taskTitled(prefix string) *fakeTask {
	for _, t := range f.tasks {
		if strings.HasPrefix(t.Title, prefix) {
			return t
		}
	}
	return nil
}

func labelTitles(t *fakeTask) string {
	var s []string
	for _, l := range t.Labels {
		s = append(s, l.Title)
	}
	sort.Strings(s)
	return strings.Join(s, ",")
}

func newBoardTestPipeline(t *testing.T, url string) *pipeline {
	t.Helper()
	pl, err := newPipeline(t.TempDir(), false, false)
	if err != nil {
		t.Fatal(err)
	}
	pl.log = io.Discard
	creds := boardCreds{URL: url, Owner: "noah", Token: "owner-tok", Bots: map[string]boardBot{}}
	for _, b := range boardBots {
		creds.Bots[b] = boardBot{Token: "tok-" + b}
	}
	pl.vk = &boardSyncer{pl: pl, creds: creds, owner: newVK(url, creds.Token), warned: map[string]bool{},
		stPath: filepath.Join(t.TempDir(), "board.json")}
	return pl
}

func TestBoardPush_MirrorsCardsOntoTheBoard(t *testing.T) {
	f, srv := newFakeVikunja(t)
	pl := newBoardTestPipeline(t, srv.URL)
	note := func(by, stage, done string) cardNote {
		return cardNote{TS: "2026-10-11T16:00:00Z", By: by, Stage: stage, Done: done, Why: "because", Next: "next"}
	}
	for _, c := range []card{
		{ID: "C0001", Title: "Simple pomodoro timer", Type: "feature", Stage: "tracking", Context: "hobby",
			Notes: []cardNote{note("human", "→ new", "Request created.")}},
		{ID: "C0002", Title: "Core Timer Engine", Type: "story", Stage: "review", Parent: "C0001", Context: "hobby",
			Artifacts: map[string]string{"prs": "https://github.com/weyenk/ai-mode/pull/19"},
			Notes:     []cardNote{note("bot-architect", "→ amigos", "Story cut."), note("operator (Claude)", "review", "A correction.")}},
		{ID: "C0003", Title: "CLI", Type: "story", Stage: "needs-human", Parent: "C0001", DependsOn: []string{"C0002"},
			Notes: []cardNote{note("bot-chief", "amigos → needs-human", "Stopped.")}},
	} {
		if err := pl.board.save(c); err != nil {
			t.Fatal(err)
		}
	}
	if err := pl.vk.push(); err != nil {
		t.Fatal(err)
	}

	var pr *vkProject
	for _, p := range f.projects {
		pr = p
	}
	if pr == nil || len(f.shares[pr.ID]) != len(boardBots) {
		t.Fatalf("project %+v shared with %v", pr, f.shares)
	}
	var cols []string
	bs := f.buckets[pr.Views[1].ID]
	sort.Slice(bs, func(i, j int) bool { return bs[i].Position < bs[j].Position })
	for _, b := range bs {
		cols = append(cols, b.Title)
	}
	if got := strings.Join(cols, " "); got != "init ideation building review done blocked" {
		t.Fatalf("buckets = %s (the three default buckets are renamed, not left behind)", got)
	}
	feature, story, cli := f.taskTitled("C0001 "), f.taskTitled("C0002 Core"), f.taskTitled("C0003 ")
	if feature == nil || story == nil || cli == nil {
		t.Fatalf("tasks: %v", f.tasks)
	}
	for task, want := range map[*fakeTask]string{feature: "review", story: "review", cli: "blocked"} {
		if got := f.bucketTitle(task.bucket); got != want {
			t.Errorf("%s in %s, want %s", task.Title, got, want)
		}
	}
	if got := labelTitles(story); got != "context:hobby,stage:review,type:story" {
		t.Errorf("story labels = %s", got)
	}
	if story.creator != "bot-architect" || len(story.comments) != 2 || story.comments[0].author != "bot-architect" || story.comments[1].author != "noah" {
		t.Fatalf("story created by %s, comments %+v", story.creator, story.comments)
	}
	if !strings.Contains(story.comments[1].html, "operator (Claude)") {
		t.Errorf("a note posted with the owner's token must say who wrote it: %s", story.comments[1].html)
	}
	if !strings.Contains(story.Description, `href="https://github.com/weyenk/ai-mode/pull/19"`) {
		t.Errorf("description lacks the PR link: %s", story.Description)
	}
	if story.rels["parenttask"][0] != feature.ID || cli.rels["follows"][0] != story.ID {
		t.Fatalf("relations: story %v, cli %v", story.rels, cli.rels)
	}
	saved, _ := pl.board.load("C0002")
	if saved.BoardTask != story.ID || saved.BoardNotes != 2 {
		t.Fatalf("card keeps its task and note count: %+v", saved)
	}

	// A second push changes nothing; a stage change moves the task, swaps the label and adds one comment.
	if err := pl.vk.push(); err != nil {
		t.Fatal(err)
	}
	if len(story.comments) != 2 || len(story.rels["parenttask"]) != 1 || len(f.tasks) != 3 {
		t.Fatalf("second push duplicated: %d comments, rels %v, %d tasks", len(story.comments), story.rels, len(f.tasks))
	}
	c3, _ := pl.board.load("C0003")
	c3.Stage = "amigos"
	c3.Notes = append(c3.Notes, note("human", "needs-human → amigos", "Retry requested."))
	pl.board.save(c3)
	if err := pl.vk.push(); err != nil {
		t.Fatal(err)
	}
	if got := f.bucketTitle(cli.bucket); got != "building" {
		t.Errorf("retried card in %s", got)
	}
	if got := labelTitles(cli); got != "stage:amigos,type:story" {
		t.Errorf("labels after the move = %s", got)
	}
	if len(cli.comments) != 2 {
		t.Errorf("comments = %d", len(cli.comments))
	}
}

func TestBoardPull_IntakeFromInitAndDoneFromAHuman(t *testing.T) {
	f, srv := newFakeVikunja(t)
	pl := newBoardTestPipeline(t, srv.URL)
	story := card{ID: "C0001", Title: "Core Timer Engine", Type: "story", Stage: "review",
		Notes: []cardNote{{By: "bot-chief", Stage: "handoff → review", Done: "PRs opened."}}}
	pl.board.save(story)
	if err := pl.vk.push(); err != nil {
		t.Fatal(err)
	}
	st := pl.vk.st
	story, _ = pl.board.load("C0001")
	f.tasks[story.BoardTask].bucket = st.Buckets["done"] // a human merged and moved it

	bug := &fakeTask{vkTask: vkTask{ID: f.id(), Title: "Bell is too quiet", Description: "<p>Make it <b>ring</b> &amp; flash</p><p>twice</p>",
		ProjectID: st.Project, Labels: []vkLabel{{Title: "type:bug"}, {Title: "context:enterprise"}}},
		bucket: st.Buckets["init"], creator: "noah", rels: map[string][]int64{}}
	f.tasks[bug.ID] = bug

	if err := pl.vk.pull(); err != nil {
		t.Fatal(err)
	}
	done, _ := pl.board.load("C0001")
	if done.Stage != "done" || done.Notes[len(done.Notes)-1].By != "human (on the board)" {
		t.Fatalf("story after the human moved it: %s, %+v", done.Stage, done.Notes[len(done.Notes)-1])
	}
	cards, _ := pl.board.all()
	var req *card
	for i := range cards {
		if cards[i].BoardTask == bug.ID {
			req = &cards[i]
		}
	}
	if req == nil || req.Type != "bug" || req.Context != "enterprise" || req.Stage != "new" || req.Notes[0].By != "noah (on the board)" {
		t.Fatalf("intake card = %+v", req)
	}
	body, _ := os.ReadFile(pl.abs(req.Artifacts["request"]))
	if !strings.Contains(string(body), "Make it ring & flash\n\ntwice") {
		t.Fatalf("request file:\n%s", body)
	}
	// pull also pushed: the new task got its card id and its first note.
	if !strings.HasPrefix(bug.Title, req.ID+" ") || len(bug.comments) != 1 {
		t.Fatalf("task after sync: %q, %d comments", bug.Title, len(bug.comments))
	}
	// A second pull does not take the same task in again.
	if err := pl.vk.pull(); err != nil {
		t.Fatal(err)
	}
	if again, _ := pl.board.all(); len(again) != len(cards) {
		t.Fatalf("cards %d → %d", len(cards), len(again))
	}
}

func TestPickPermissions_KeepsWhatTheServerOffers(t *testing.T) {
	avail := map[string]map[string]json.RawMessage{
		"tasks":  {"read_one": nil, "create": nil, "delete": nil},
		"labels": {"read_all": nil, "create": nil},
	}
	got := pickPermissions(avail, map[string][]string{"tasks": {"read_one", "update"}, "labels": {"*"}, "teams": {"read_all"}})
	b, _ := json.Marshal(got)
	if string(b) != `{"labels":["create","read_all"],"tasks":["read_one"]}` {
		t.Fatalf("got %s", b)
	}
}

func TestGitHubRemotePattern(t *testing.T) {
	for remote, want := range map[string]string{
		"git@github.com:weyenk/ai-mode.git":     "weyenk/ai-mode",
		"https://github.com/weyenk/ai-mode":     "weyenk/ai-mode",
		"https://gitlab.com/weyenk/ai-mode.git": "",
	} {
		got := ""
		if m := reGitHubRemote.FindStringSubmatch(remote); m != nil {
			got = m[1] + "/" + m[2]
		}
		if got != want {
			t.Errorf("%s → %q, want %q", remote, got, want)
		}
	}
}
