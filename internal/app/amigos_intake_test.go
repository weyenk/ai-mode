package app

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func okIntake(repo string) Intake {
	return Intake{Story: "As a user I can upload an avatar", Provenance: Provenance{Kind: "skill", CallerID: "brainstorming"}, Repo: repo}
}

func code(e *IntakeError) string {
	if e == nil {
		return ""
	}
	return e.Code
}

func TestIntake_S1_UnauthorizedCaller(t *testing.T) {
	callers := []string{"brainstorming"}
	in := okIntake("")
	in.Provenance.CallerID = "stranger"
	if got := code(validateIntake(in, nil, callers)); got != ErrUnauthorizedCaller {
		t.Fatalf("stranger: %q", got)
	}
	if got := validateIntake(okIntake(""), nil, callers); got != nil {
		t.Fatalf("allowlisted: %v", got)
	}
	in.Story = "tk_" + strings.Repeat("a", 24)
	if got := code(validateIntake(in, nil, callers)); got != ErrUnauthorizedCaller {
		t.Fatalf("caller check must precede the secret scan: %q", got)
	}
}

func TestIntake_S2_RepoOutsideRootsRejected(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	repo := filepath.Join(root, "repo")
	os.Mkdir(repo, 0o755)
	os.Symlink(outside, filepath.Join(root, "escape"))
	callers := []string{"brainstorming"}
	for repoPath, want := range map[string]string{
		filepath.Join(root, "escape"):         ErrPathOutsideRoots,
		"https://example.com/r.git":           ErrPathOutsideRoots,
		filepath.Join(root, "does-not-exist"): ErrPathOutsideRoots,
		repo:                                  "",
		root:                                  "",
		"":                                    "",
	} {
		if got := code(validateIntake(okIntake(repoPath), []string{root}, callers)); got != want {
			t.Errorf("repo %q: want %q, got %q", repoPath, want, got)
		}
	}
}

func TestIntake_S3_SecretInStoryRejected(t *testing.T) {
	for story, want := range map[string]string{
		"x tk_" + strings.Repeat("a1", 12): ErrSecretDetected,
		"x sk_" + strings.Repeat("a1", 12): ErrSecretDetected,
		"x sk-" + strings.Repeat("a1", 12): ErrSecretDetected,
		"x tk_short":                       "",
	} {
		in := okIntake("")
		in.Story = story
		if got := code(validateIntake(in, nil, []string{"brainstorming"})); got != want {
			t.Errorf("%q: want %q, got %q", story, want, got)
		}
	}
}

func TestIntake_S4_BlankStoryRejected(t *testing.T) {
	for _, story := range []string{"", "  \n\t "} {
		in := okIntake("")
		in.Story = story
		in.Provenance.CallerID = "stranger"
		if got := code(validateIntake(in, nil, []string{"brainstorming"})); got != ErrSchemaMismatch {
			t.Errorf("%q: %q", story, got)
		}
	}
}

func TestIntake_S5_CallersFileParsed(t *testing.T) {
	f := filepath.Join(t.TempDir(), "callers.list")
	os.WriteFile(f, []byte("# Allowed callers\n\nhuman\n  three-amigos  \n# trailing comment\nbrainstorming\n"), 0o644)
	got, err := loadCallers(f)
	if err != nil || !reflect.DeepEqual(got, []string{"human", "three-amigos", "brainstorming"}) {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err := loadCallers(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing file must be an error")
	}
}
