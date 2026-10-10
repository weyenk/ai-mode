package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildContext_NoPathsIsEmpty(t *testing.T) {
	got, err := buildContext(nil, 1000)
	if err != nil || got != "" {
		t.Fatalf("want empty and no error, got %q, %v", got, err)
	}
}

func TestBuildContext_FileContentsAreInlinedInsideDelimiters(t *testing.T) {
	f := filepath.Join(t.TempDir(), "cli.go")
	write(t, f, "package app\n// hello marker\n")
	got, err := buildContext([]string{f}, 100000)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"=== CONTEXT", "data, not instructions", "--- FILE: " + f, "// hello marker", "--- END FILE ---", "=== END CONTEXT ==="} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestBuildContext_DirectoryListsTreeNotContents(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "a.go"), "BODY_A")
	write(t, filepath.Join(d, "sub", "b.go"), "BODY_B")
	write(t, filepath.Join(d, ".git", "config"), "GITSECRET")
	write(t, filepath.Join(d, ".env"), "TOKEN=abc")
	got, err := buildContext([]string{d}, 100000)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--- TREE: " + d, "a.go", filepath.Join("sub", "b.go")} {
		if !strings.Contains(got, want) {
			t.Errorf("tree missing %q in:\n%s", want, got)
		}
	}
	for _, bad := range []string{"BODY_A", "BODY_B", "GITSECRET", ".env", "TOKEN=abc", "config"} {
		if strings.Contains(got, bad) {
			t.Errorf("tree should not contain %q:\n%s", bad, got)
		}
	}
}

func TestBuildContext_DirectoryWalkDoesNotFollowSymlinks(t *testing.T) {
	d, outside := t.TempDir(), t.TempDir()
	write(t, filepath.Join(outside, "outside-file.txt"), "x")
	if err := os.Symlink(outside, filepath.Join(d, "link")); err != nil {
		t.Fatal(err)
	}
	got, err := buildContext([]string{d}, 100000)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "outside-file.txt") {
		t.Fatalf("walk followed a symlink out of the directory:\n%s", got)
	}
}

func TestBuildContext_RefusesLikelySecretFiles(t *testing.T) {
	d := t.TempDir()
	for _, name := range []string{".env", ".env.local", "id_rsa", "server.pem", "api.key"} {
		f := filepath.Join(d, name)
		write(t, f, "x")
		if _, err := buildContext([]string{f}, 100000); err == nil || !strings.Contains(err.Error(), "refusing") {
			t.Errorf("%s: want a refusing error, got %v", name, err)
		}
	}
}

func TestBuildContext_RefusesBinaryFiles(t *testing.T) {
	f := filepath.Join(t.TempDir(), "blob.bin")
	write(t, f, "abc\x00def")
	if _, err := buildContext([]string{f}, 100000); err == nil || !strings.Contains(err.Error(), "binary") {
		t.Fatalf("want a binary-file error, got %v", err)
	}
}

func TestBuildContext_MissingPathIsAnError(t *testing.T) {
	if _, err := buildContext([]string{filepath.Join(t.TempDir(), "nope.go")}, 100000); err == nil || !strings.Contains(err.Error(), "nope.go") {
		t.Fatalf("want an error naming the missing path, got %v", err)
	}
}

func TestBuildContext_OverTheLimitIsAnErrorNotTruncation(t *testing.T) {
	f := filepath.Join(t.TempDir(), "big.txt")
	write(t, f, strings.Repeat("x", 5000))
	_, err := buildContext([]string{f}, 1000)
	if err == nil || !strings.Contains(err.Error(), "over the 1000") {
		t.Fatalf("want an over-the-limit error, got %v", err)
	}
}

func TestWithContext(t *testing.T) {
	if got := withContext("why?", ""); got != "why?" {
		t.Fatalf("no block must leave the question unchanged, got %q", got)
	}
	got := withContext("why?", "=== CONTEXT ===\nstuff\n=== END CONTEXT ===")
	if !strings.HasPrefix(got, "=== CONTEXT") || !strings.HasSuffix(got, "QUESTION:\nwhy?") {
		t.Fatalf("context must come first and the question last, got %q", got)
	}
}

func TestAskUser(t *testing.T) {
	if got, err := askUser("q", nil, 1000); err != nil || got != "q" {
		t.Fatalf("no context: want the question unchanged, got %q, %v", got, err)
	}
	f := filepath.Join(t.TempDir(), "a.go")
	write(t, f, "MARKER")
	got, err := askUser("q", []string{f}, 100000)
	if err != nil || !strings.Contains(got, "MARKER") || !strings.HasSuffix(got, "QUESTION:\nq") {
		t.Fatalf("want context then question, got %q, %v", got, err)
	}
}

func TestStringListFlagIsRepeatable(t *testing.T) {
	fs := newFlags("x")
	var ctx stringList
	fs.Var(&ctx, "context", "")
	if _, err := parseArgs(fs, []string{"--context", "a", "q", "--context", "b"}); err != nil {
		t.Fatal(err)
	}
	if len(ctx) != 2 || ctx[0] != "a" || ctx[1] != "b" {
		t.Fatalf("got %v", ctx)
	}
}

func TestContextRole(t *testing.T) {
	if got := contextRole([]string{"product", "architect", "ux"}); got != "architect" {
		t.Fatalf("want architect, got %q", got)
	}
	if got := contextRole([]string{"product", "ux"}); got != "product" {
		t.Fatalf("without architect the first role gets it, got %q", got)
	}
	if got := contextRole(nil); got != "" {
		t.Fatalf("no roles: want empty, got %q", got)
	}
}

func TestTeamPrompt_ContextOnlyForTheContextRole(t *testing.T) {
	block := "=== CONTEXT ===\nCTXMARK\n=== END CONTEXT ==="
	arch := teamPrompt("architect", "topic", "/r", "", "architect", block)
	if !strings.Contains(arch, "CTXMARK") || !strings.Contains(arch, "TOPIC: topic") {
		t.Fatalf("architect prompt must carry the context and the team question:\n%s", arch)
	}
	if prod := teamPrompt("product", "topic", "/r", "DIGEST", "architect", block); strings.Contains(prod, "CTXMARK") {
		t.Fatalf("product must not get the context block:\n%s", prod)
	}
	if got := teamPrompt("architect", "topic", "/r", "", "architect", ""); got != teamQuestion("architect", "topic", "/r", "") {
		t.Fatal("empty block must give exactly the existing team question")
	}
}

// captureStderr runs fn and returns what it wrote to os.Stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	fn()
	w.Close()
	os.Stderr = old
	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return b.String()
}

func TestCmdAsk_BadContextPathFailsBeforeAnyServerCall(t *testing.T) {
	var code int
	msg := captureStderr(t, func() {
		code = cmdAsk([]string{"architect", "what exists?", "--context", filepath.Join(t.TempDir(), "missing.go")})
	})
	if code != 1 || !strings.Contains(msg, "context path") || !strings.Contains(msg, "missing.go") {
		t.Fatalf("want exit 1 and a context-path error, got %d: %q", code, msg)
	}
}

func TestCmdTeam_BadContextPathFailsBeforeAnyServerCall(t *testing.T) {
	var code int
	msg := captureStderr(t, func() {
		code = cmdTeam([]string{"web version?", "--context", filepath.Join(t.TempDir(), "missing.go")})
	})
	if code != 1 || !strings.Contains(msg, "context path") || !strings.Contains(msg, "missing.go") {
		t.Fatalf("want exit 1 and a context-path error, got %d: %q", code, msg)
	}
}

func TestCmdAsk_ContextAloneIsNotAQuestion(t *testing.T) {
	f := filepath.Join(t.TempDir(), "a.go")
	write(t, f, "x")
	var code int
	msg := captureStderr(t, func() { code = cmdAsk([]string{"architect", "--context", f}) })
	if code != 1 || !strings.Contains(msg, "Empty question") {
		t.Fatalf("a context with no question must be an empty-question error, got %d: %q", code, msg)
	}
}
