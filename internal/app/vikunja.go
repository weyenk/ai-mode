package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// A thin stdlib client for Vikunja's /api/v2 (the work board, roadmap P1; decisions and trial results in
// docs/superpowers/specs/2026-10-10-work-board-design.md). Routes and bodies were checked against the v2.7.0
// OpenAPI spec the server serves at /api/v2/openapi.json. Vikunja runs on its own (launchd, Postgres); ai-mode only
// talks to its API. With Postgres concurrent writes are safe, so there is no write gate; a 5xx is retried a few times
// in case the board runs on SQLite, which answers "database is locked" under concurrent writers.

type vkClient struct {
	base  string // http://127.0.0.1:3456, no trailing slash
	token string // a user JWT or an API token (tk_…)
	hc    *http.Client
}

func newVK(base, token string) *vkClient {
	return &vkClient{base: strings.TrimRight(base, "/"), token: token, hc: &http.Client{Timeout: 20 * time.Second}}
}

// vkError is an RFC 9457 problem body plus the status line.
type vkError struct {
	Status int
	Method string
	Path   string
	Detail string
}

func (e *vkError) Error() string {
	return fmt.Sprintf("vikunja %s %s: %d %s", e.Method, e.Path, e.Status, e.Detail)
}

func vkStatus(err error) int {
	var e *vkError
	if errors.As(err, &e) {
		return e.Status
	}
	return 0
}

func (c *vkClient) do(method, path string, body, out any) error {
	return c.send(method, path, "application/json", body, out)
}

// patch sends a JSON Merge Patch: only the fields in body change.
func (c *vkClient) patch(path string, body, out any) error {
	return c.send(http.MethodPatch, path, "application/merge-patch+json", body, out)
}

func (c *vkClient) send(method, path, ctype string, body, out any) error {
	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = b
	}
	var last error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*250) * time.Millisecond)
		}
		req, err := http.NewRequest(method, c.base+"/api/v2"+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", ctype)
		}
		if c.token != "" {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}
		resp, err := c.hc.Do(req)
		if err != nil {
			return fmt.Errorf("vikunja %s %s: %w", method, path, err)
		}
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if resp.StatusCode == http.StatusNotModified {
			return nil
		}
		if resp.StatusCode >= 400 {
			var p struct {
				Title   string `json:"title"`
				Detail  string `json:"detail"`
				Message string `json:"message"`
			}
			_ = json.Unmarshal(data, &p)
			detail := firstNonEmpty(p.Detail, p.Message, p.Title, strings.TrimSpace(string(data)))
			last = &vkError{Status: resp.StatusCode, Method: method, Path: path, Detail: truncateRunes(detail, 300)}
			if resp.StatusCode >= 500 {
				continue
			}
			return last
		}
		if out != nil && len(data) > 0 {
			if err := json.Unmarshal(data, out); err != nil {
				return fmt.Errorf("vikunja %s %s: decode: %w", method, path, err)
			}
		}
		return nil
	}
	return last
}

// vkList reads every page of a v2 list (the {items, total_pages} envelope).
func vkList[T any](c *vkClient, path string) ([]T, error) {
	var all []T
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	for page := 1; page <= 200; page++ {
		var env struct {
			Items      []T   `json:"items"`
			TotalPages int64 `json:"total_pages"`
		}
		if err := c.do(http.MethodGet, fmt.Sprintf("%s%spage=%d&per_page=50", path, sep, page), nil, &env); err != nil {
			return nil, err
		}
		all = append(all, env.Items...)
		if int64(page) >= env.TotalPages || len(env.Items) == 0 {
			break
		}
	}
	return all, nil
}

type vkUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name,omitempty"`
}

type vkToken struct {
	ID          int64               `json:"id,omitempty"`
	Title       string              `json:"title"`
	Token       string              `json:"token,omitempty"`
	Permissions map[string][]string `json:"permissions"`
	ExpiresAt   time.Time           `json:"expires_at"`
	OwnerID     int64               `json:"owner_id,omitempty"`
}

type vkProject struct {
	ID          int64    `json:"id,omitempty"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	IsArchived  bool     `json:"is_archived,omitempty"`
	Views       []vkView `json:"views,omitempty"`
}

type vkView struct {
	ID              int64  `json:"id"`
	Title           string `json:"title"`
	ViewKind        string `json:"view_kind"`
	DefaultBucketID int64  `json:"default_bucket_id"`
	DoneBucketID    int64  `json:"done_bucket_id"`
}

type vkBucket struct {
	ID       int64    `json:"id,omitempty"`
	Title    string   `json:"title"`
	Position float64  `json:"position"`
	Limit    int64    `json:"limit"`
	Tasks    []vkTask `json:"tasks,omitempty"`
}

type vkLabel struct {
	ID          int64  `json:"id,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	HexColor    string `json:"hex_color,omitempty"`
}

type vkTask struct {
	ID           int64               `json:"id,omitempty"`
	Title        string              `json:"title"`
	Description  string              `json:"description"`
	ProjectID    int64               `json:"project_id,omitempty"`
	BucketID     int64               `json:"bucket_id,omitempty"`
	Done         bool                `json:"done,omitempty"`
	Labels       []vkLabel           `json:"labels,omitempty"`
	CreatedBy    *vkUser             `json:"created_by,omitempty"`
	RelatedTasks map[string][]vkTask `json:"related_tasks,omitempty"`
}

// vkFarFuture is how Vikunja's own agent CLI (veans) asks for a token that does not expire.
var vkFarFuture = time.Date(9999, time.December, 31, 0, 0, 0, 0, time.UTC)

func (c *vkClient) login(username, password string) (string, error) {
	var out struct {
		Token string `json:"token"`
	}
	err := c.do(http.MethodPost, "/login", map[string]any{"username": username, "password": password, "long_token": false}, &out)
	return out.Token, err
}

func (c *vkClient) register(username, email, password string) error {
	return c.do(http.MethodPost, "/register", map[string]string{"username": username, "email": email, "password": password}, nil)
}

func (c *vkClient) me() (vkUser, error) {
	var u vkUser
	return u, c.do(http.MethodGet, "/user", nil, &u)
}

// routes lists what an API token may be granted: group -> action -> route.
func (c *vkClient) routes() (map[string]map[string]json.RawMessage, error) {
	out := map[string]map[string]json.RawMessage{}
	return out, c.do(http.MethodGet, "/routes", nil, &out)
}

func (c *vkClient) bots() ([]vkUser, error) { return vkList[vkUser](c, "/user/bots") }

func (c *vkClient) createBot(username, name string) (vkUser, error) {
	var u vkUser
	return u, c.do(http.MethodPost, "/user/bots", map[string]string{"username": username, "name": name}, &u)
}

func (c *vkClient) createToken(t vkToken) (vkToken, error) {
	var out vkToken
	return out, c.do(http.MethodPost, "/tokens", t, &out)
}

func (c *vkClient) projects() ([]vkProject, error) { return vkList[vkProject](c, "/projects") }

func (c *vkClient) project(id int64) (vkProject, error) {
	var p vkProject
	return p, c.do(http.MethodGet, fmt.Sprintf("/projects/%d", id), nil, &p)
}

func (c *vkClient) createProject(p vkProject) (vkProject, error) {
	var out vkProject
	return out, c.do(http.MethodPost, "/projects", p, &out)
}

func (c *vkClient) shareProject(project int64, username string, permission int) error {
	return c.do(http.MethodPost, fmt.Sprintf("/projects/%d/users", project), map[string]any{"username": username, "permission": permission}, nil)
}

func (c *vkClient) views(project int64) ([]vkView, error) {
	return vkList[vkView](c, fmt.Sprintf("/projects/%d/views", project))
}

func (c *vkClient) buckets(project, view int64) ([]vkBucket, error) {
	return vkList[vkBucket](c, fmt.Sprintf("/projects/%d/views/%d/buckets", project, view))
}

func (c *vkClient) createBucket(project, view int64, b vkBucket) (vkBucket, error) {
	var out vkBucket
	return out, c.do(http.MethodPost, fmt.Sprintf("/projects/%d/views/%d/buckets", project, view), b, &out)
}

func (c *vkClient) updateBucket(project, view int64, b vkBucket) error {
	return c.do(http.MethodPut, fmt.Sprintf("/projects/%d/views/%d/buckets/%d", project, view, b.ID), b, nil)
}

// bucketTasks is a kanban view's buckets, each with its tasks.
func (c *vkClient) bucketTasks(project, view int64) ([]vkBucket, error) {
	var env struct {
		Items []vkBucket `json:"items"`
	}
	return env.Items, c.do(http.MethodGet, fmt.Sprintf("/projects/%d/views/%d/buckets/tasks", project, view), nil, &env)
}

func (c *vkClient) labels() ([]vkLabel, error) { return vkList[vkLabel](c, "/labels") }

func (c *vkClient) createLabel(l vkLabel) (vkLabel, error) {
	var out vkLabel
	return out, c.do(http.MethodPost, "/labels", l, &out)
}

func (c *vkClient) task(id int64) (vkTask, error) {
	var t vkTask
	return t, c.do(http.MethodGet, fmt.Sprintf("/tasks/%d", id), nil, &t)
}

func (c *vkClient) createTask(project int64, t vkTask) (vkTask, error) {
	var out vkTask
	return out, c.do(http.MethodPost, fmt.Sprintf("/projects/%d/tasks", project), t, &out)
}

func (c *vkClient) patchTask(id int64, fields map[string]any) error {
	return c.patch(fmt.Sprintf("/tasks/%d", id), fields, nil)
}

func (c *vkClient) moveTask(project, view, bucket, task int64) error {
	return c.do(http.MethodPut, fmt.Sprintf("/projects/%d/views/%d/buckets/%d/tasks", project, view, bucket),
		map[string]int64{"task_id": task}, nil)
}

func (c *vkClient) addLabel(task, label int64) error {
	return c.do(http.MethodPost, fmt.Sprintf("/tasks/%d/labels", task), map[string]int64{"label_id": label}, nil)
}

func (c *vkClient) removeLabel(task, label int64) error {
	return c.do(http.MethodDelete, fmt.Sprintf("/tasks/%d/labels/%d", task, label), nil, nil)
}

func (c *vkClient) comment(task int64, html string) error {
	return c.do(http.MethodPost, fmt.Sprintf("/tasks/%d/comments", task), map[string]string{"comment": html}, nil)
}

func (c *vkClient) relate(task, other int64, kind string) error {
	return c.do(http.MethodPost, fmt.Sprintf("/tasks/%d/relations", task), map[string]any{"other_task_id": other, "relation_kind": kind}, nil)
}

// info is the unauthenticated server description; doctor uses it as the health check.
func vkInfo(base string) (map[string]any, error) {
	var out map[string]any
	c := newVK(base, "")
	c.hc.Timeout = 3 * time.Second
	return out, c.do(http.MethodGet, "/info", nil, &out)
}
