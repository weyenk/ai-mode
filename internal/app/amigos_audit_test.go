package app

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeAll(t *testing.T, s auditSink) {
	t.Helper()
	snap := Snapshot{Contract: StoryContract{Story: "s"}, State: MeetingState{MeetingID: "m1"}}
	for kind, v := range map[string]any{
		"contract":   snap,
		"decision":   DecisionEntry{Round: 1, Role: "qa"},
		"parked":     Question{Text: "Q1", State: QuestionParked},
		"transcript": map[string]string{"role": "qa", "prompt": "p", "reply": "r"},
	} {
		if err := s.Write(kind, v); err != nil {
			t.Fatalf("write %s: %v", kind, err)
		}
	}
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func TestAudit_S7_LevelsWriteWhatTheyShould(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "off")
	s, err := newFileAudit("off", dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	writeAll(t, s)
	if exists(dir) {
		t.Fatal("level off must not create the directory")
	}
	for _, lv := range []string{"summary", "full"} {
		dir := filepath.Join(base, lv)
		s, err := newFileAudit(lv, dir, 0)
		if err != nil {
			t.Fatal(err)
		}
		writeAll(t, s)
		for _, f := range []string{"contract.json", "decisions.jsonl", "parked.jsonl"} {
			if !exists(filepath.Join(dir, f)) {
				t.Errorf("%s: %s missing", lv, f)
			}
		}
		if got := exists(filepath.Join(dir, "transcript.jsonl")); got != (lv == "full") {
			t.Errorf("%s: transcript.jsonl exists=%v", lv, got)
		}
		if st, _ := os.Stat(dir); st.Mode().Perm() != 0o700 {
			t.Errorf("%s: dir mode %v", lv, st.Mode().Perm())
		}
		if st, _ := os.Stat(filepath.Join(dir, "contract.json")); st.Mode().Perm() != 0o600 {
			t.Errorf("%s: file mode %v", lv, st.Mode().Perm())
		}
		if err := s.Write("bogus", 1); err == nil {
			t.Errorf("%s: unknown kind must be an error", lv)
		}
	}
	if _, err := newFileAudit("loud", filepath.Join(base, "x"), 0); err == nil {
		t.Fatal("bad level must be an error")
	}
}

func TestAudit_S8_ResumeLoadsState(t *testing.T) {
	base := t.TempDir()
	snap := Snapshot{
		Contract: StoryContract{Story: "s", Rules: []string{"r1"}, Questions: []Question{{Text: "Q1", State: QuestionParked}}},
		State:    MeetingState{MeetingID: "m1", Round: 1, NextRole: "product"},
	}
	s, _ := newFileAudit("summary", filepath.Join(base, "m1"), 0)
	if err := s.Write("contract", snap); err != nil {
		t.Fatal(err)
	}
	got, err := loadSnapshot(base, "m1")
	if err != nil || !reflect.DeepEqual(got, snap) {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, err := loadSnapshot(base, "nope"); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("unknown id: %v", err)
	}
	for _, bad := range []string{"../m1", ""} {
		if _, err := loadSnapshot(base, bad); err == nil {
			t.Errorf("id %q must be rejected", bad)
		}
	}
	os.MkdirAll(filepath.Join(base, "m2"), 0o700)
	data, _ := os.ReadFile(filepath.Join(base, "m1", "contract.json"))
	os.WriteFile(filepath.Join(base, "m2", "contract.json"), data, 0o600)
	if _, err := loadSnapshot(base, "m2"); err == nil {
		t.Fatal("a contract.json that belongs to another meeting must be rejected")
	}
}

func TestAudit_S9_SecretsRedactedInTranscript(t *testing.T) {
	dir := t.TempDir()
	s, _ := newFileAudit("full", dir, 0)
	tk, sk := "tk_"+strings.Repeat("a1", 12), "sk-"+strings.Repeat("Z9", 12)
	if err := s.Write("transcript", map[string]string{"role": "qa", "prompt": "p", "reply": "keys " + tk + " and " + sk}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "transcript.jsonl"))
	if !strings.Contains(string(data), "[REDACTED]") || strings.Contains(string(data), tk) || strings.Contains(string(data), sk) {
		t.Fatalf("transcript not redacted: %s", data)
	}
	if got := redactSecrets("tk_short and sk_" + strings.Repeat("b2", 12)); got != "tk_short and [REDACTED]" {
		t.Fatalf("redactSecrets: %q", got)
	}
}

func TestAudit_S30_SizeCapStopsWrites(t *testing.T) {
	dir := t.TempDir()
	s, _ := newFileAudit("full", dir, 300)
	var err error
	for i := 0; i < 50 && err == nil; i++ {
		err = s.Write("decision", DecisionEntry{Round: i, Role: "qa", Decision: "x"})
	}
	if !errors.Is(err, errAuditQuota) {
		t.Fatalf("want errAuditQuota, got %v", err)
	}
	if st, _ := os.Stat(filepath.Join(dir, "decisions.jsonl")); st.Size() > 300 {
		t.Fatalf("file grew past the cap: %d", st.Size())
	}
	u, _ := newFileAudit("full", t.TempDir(), 0)
	for i := 0; i < 50; i++ {
		if err := u.Write("decision", DecisionEntry{Round: i}); err != nil {
			t.Fatalf("no cap: %v", err)
		}
	}
}
