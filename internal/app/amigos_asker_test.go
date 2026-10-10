package app

import (
	"testing"
)

func TestAsker_S16_PassesArgsAndSurfacesFailure(t *testing.T) {
	var captured askParams
	// Success case: check that arguments are passed correctly.
	fSuccess := func(a askParams) (string, int) {
		captured = a
		return "hello", 0
	}

	p := Profile{Name: "test-profile"}
	caller := "test-caller"
	asker := newRoleAsker(p, caller, fSuccess)

	txt, err := asker.Ask("product", "As a user...", 1000)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if txt != "hello" {
		t.Errorf("expected text 'hello', got %q", txt)
	}
	if captured.role != "product" {
		t.Errorf("expected role 'product', got %q", captured.role)
	}
	if captured.user != "As a user..." {
		t.Errorf("expected user 'As a user...', got %q", captured.user)
	}
	if captured.maxTokens != 1000 {
		t.Errorf("expected maxTokens 1000, got %d", captured.maxTokens)
	}
	if captured.caller != caller {
		t.Errorf("expected caller %q, got %q", caller, captured.caller)
	}
	if captured.timeout <= 0 {
		t.Errorf("expected a positive timeout, got %v", captured.timeout)
	}

	// Failure case: check that a non-zero exit code surfaces an error.
	fFailure := func(a askParams) (string, int) {
		return "", 1
	}
	askerErr := newRoleAsker(p, caller, fFailure)
	_, err = askerErr.Ask("qa", "some question", 500)
	if err == nil {
		t.Fatal("expected error for non-zero exit code, got nil")
	}
}

func TestAsker_ArchitectGetsRoomToThink(t *testing.T) {
	var got int
	a := newRoleAsker(Profile{Name: "p"}, "c", func(p askParams) (string, int) { got = p.maxTokens; return "x", 0 })
	if _, err := a.Ask("architect", "q", 2048); err != nil || got != architectMinTokens {
		t.Fatalf("architect maxTokens = %d (err %v), want %d", got, err, architectMinTokens)
	}
	if _, err := a.Ask("architect", "q", architectMinTokens+5); err != nil || got != architectMinTokens+5 {
		t.Fatalf("a larger request must be kept, got %d", got)
	}
}
