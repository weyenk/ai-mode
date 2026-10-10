package app

import "fmt"

// askTimeoutSeconds is the per-call timeout handed to runAsk (same default as `ai-mode ask`).
const askTimeoutSeconds = 600

// askFunc is the injected function used by the roleAsker adapter.
type askFunc func(a askParams) (string, int)

// amigosAsker implements the roleAsker interface by wrapping an askFunc.
type amigosAsker struct {
	p      Profile
	caller string
	ask    askFunc
}

// Ask calls the injected askFunc with the provided parameters.
func (a *amigosAsker) Ask(role, user string, maxTokens int) (string, error) {
	// architect thinks before it answers; a small cap leaves only its reasoning, not the JSON reply.
	if role == "architect" && maxTokens < architectMinTokens {
		maxTokens = architectMinTokens
	}
	txt, code := a.ask(askParams{
		profile:   a.p,
		role:      role,
		user:      user,
		maxTokens: maxTokens,
		timeout:   askTimeoutSeconds,
		caller:    a.caller,
	})
	if code != 0 {
		return "", fmt.Errorf("ask failed with code %d", code)
	}
	return txt, nil
}

// newRoleAsker creates a new roleAsker using the provided profile, caller, and ask function.
func newRoleAsker(p Profile, caller string, ask askFunc) roleAsker {
	return &amigosAsker{p: p, caller: caller, ask: ask}
}
