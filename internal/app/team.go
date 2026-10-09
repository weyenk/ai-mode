package app

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// defaultTeam is the early-exploration roster, in call order. architect goes
// first: its repo findings are passed to the others as shared context.
var defaultTeam = []string{"architect", "product", "research", "ux", "security"}

// teamBriefs is each role's angle on a topic. Roles missing here can still be
// listed in --roles; they get a generic brief.
var teamBriefs = map[string]string{
	"architect": "Explore the repository (read only what you need) and report: (1) what exists today that is relevant, with file paths; " +
		"(2) what is reusable versus what must be built; (3) 2-3 feasible technical approaches with trade-offs and a recommendation; " +
		"(4) the open questions only the human can answer. If the topic is ambiguous about which app, surface, or package it means, say so first.",
	"product": "Who is this for and what job does it do for them? What is the smallest valuable first version, what is explicitly out of scope, " +
		"how would we know it worked, and which open questions would change scope?",
	"research": "What prior art, standard approaches, platforms and technologies are relevant? What is possible and what is not? " +
		"Name known pitfalls. You cannot browse: flag anything uncertain that must be checked against current docs.",
	"ux":       "Which user flows and interactions change for this surface? What platform conventions, accessibility needs and likely friction points matter? Top risks first.",
	"security": "Threat-model this: assets, trust boundaries, auth and data exposure, privacy or compliance implications. Top risks with a mitigation each.",
	"design":   "What visual and layout decisions does this force (responsive behaviour, components, theming, brand consistency)? What should be decided before building?",
	"qa":       "How would this be tested? Name the layers that matter, the riskiest behaviours, and what is hard to test.",
	"docs":     "What documentation does this need (user, developer, migration), and what must be written down early to avoid rework?",
}

// architectMinTokens leaves room for architect's thinking (reasoning-budget in
// the preset) plus an answer; specialists run with thinking off.
const architectMinTokens = 10240

// contextDigestChars caps the architect findings forwarded to other roles
// (some have a 16k window).
const contextDigestChars = 4000

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(s) > 48 {
		s = strings.Trim(s[:48], "-")
	}
	if s == "" {
		s = "topic"
	}
	return s
}

func truncateLines(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := s[:max]
	if i := strings.LastIndex(cut, "\n"); i > max/2 {
		cut = cut[:i]
	}
	return strings.TrimSpace(cut) + "\n…(truncated)"
}

func teamQuestion(role, topic, cwd, architectDigest string) string {
	brief := teamBriefs[role]
	if brief == "" {
		brief = "Give your domain's view of this topic: key findings, risks, and open questions."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Early team consult. Nothing is designed yet; the human wants each role's view before a design is written.\n\nTOPIC: %s\nREPO: %s\n", topic, cwd)
	if architectDigest != "" {
		fmt.Fprintf(&b, "\nARCHITECT'S REPO FINDINGS (shared context; do not repeat or re-verify):\n%s\n", architectDigest)
	}
	fmt.Fprintf(&b, "\nYOUR ANGLE (%s): %s\n\nAnswer under three headings: Findings, Risks, Questions for the human. Stay in your domain, be concrete, under 400 words. Mark anything you are guessing about this repo or product as \"Assumption:\"; never present a guess as a fact.", role, brief)
	return b.String()
}

func cmdTeam(args []string) int {
	fs := newFlags("team")
	profile := fs.String("profile", "", "Profile (default: active)")
	rolesF := fs.String("roles", "", "Comma-separated roles in call order (default: "+strings.Join(defaultTeam, ",")+"; roles absent from the profile are skipped)")
	maxTokens := fs.Int("max-tokens", 2048, "Max tokens per specialist (architect gets at least "+fmt.Sprint(architectMinTokens)+" for thinking)")
	timeout := fs.Float64("timeout", 600, "Per-call timeout seconds")
	out := fs.String("out", "", "Also write the report to this file (default: <state>/team/<time>-<topic>.md)")
	traceF := fs.String("trace", "", "Trace id (default: $AI_MODE_TRACE_ID or new)")
	parentF := fs.String("parent", "", "Parent span id (default: $AI_MODE_PARENT_SPAN)")
	callerF := fs.String("caller", "", "Who is asking, e.g. chief (default: $AI_MODE_CALLER or cli)")
	verbose := fs.Bool("v", false, "Print trace/span ids and timings to stderr")
	pos, code, ok := parse(fs, args)
	if !ok {
		return code
	}
	topic := readUserMessage(pos)
	if topic == "" {
		return fail("usage: ai-mode team <topic...> [--roles a,b,c] [--max-tokens N] [--timeout S] [--out FILE] [--profile P] [--caller NAME] [-v]\n(or pipe the topic on stdin)")
	}
	p, code := activeProfile(*profile, "Use: ai-mode use <name> or ai-mode team --profile <name>")
	if code != 0 {
		return code
	}

	wanted := defaultTeam
	if *rolesF != "" {
		wanted = splitCSV(*rolesF)
	}
	have := iniSectionsFile(p.Ini)
	var roles, skipped []string
	for _, r := range wanted {
		if r == "jev" || !contains(have, r) {
			skipped = append(skipped, r)
			continue
		}
		roles = append(roles, r)
	}
	if len(roles) == 0 {
		return fail("None of the requested roles (%s) are configured in %s", strings.Join(wanted, ","), filepathBase(p.Ini))
	}

	cwd, _ := os.Getwd()
	trace, parent := traceContext(*traceF, *parentF)
	caller := firstNonEmpty(*callerF, os.Getenv("AI_MODE_CALLER"), "cli")

	var rep strings.Builder
	fmt.Fprintf(&rep, "# Team consult: %s\n\nrepo: %s · profile: %s · trace: %s · %s\n", topic, cwd, p.Name, trace, time.Now().Format("2006-01-02 15:04"))
	if len(skipped) > 0 {
		fmt.Fprintf(&rep, "\nSkipped (not in this profile): %s\n", strings.Join(skipped, ", "))
	}

	var digest string
	answered := 0
	for i, role := range roles {
		fmt.Fprintf(os.Stderr, "[%d/%d] %s …\n", i+1, len(roles), role)
		mt := *maxTokens
		if role == "architect" && mt < architectMinTokens {
			mt = architectMinTokens
		}
		start := time.Now()
		ctxDigest := ""
		if role != "architect" {
			ctxDigest = digest
		}
		text, c := runAsk(askParams{
			profile: p, role: role, user: teamQuestion(role, topic, cwd, ctxDigest),
			maxTokens: mt, timeout: *timeout, verbose: *verbose,
			trace: trace, parent: parent, caller: caller,
		})
		fmt.Fprintf(&rep, "\n## %s\n\n", role)
		if c != 0 {
			fmt.Fprintf(&rep, "_FAILED: %s did not answer (see `ai-mode trace %s`). Do not report this role's view._\n", role, trace)
			continue
		}
		answered++
		fmt.Fprintf(os.Stderr, "[%d/%d] %s done in %s\n", i+1, len(roles), role, time.Since(start).Round(time.Second))
		fmt.Fprintf(&rep, "%s\n", strings.TrimSpace(text))
		if role == "architect" {
			digest = truncateLines(strings.TrimSpace(text), contextDigestChars)
		}
	}

	report := rep.String()
	path := *out
	if path == "" {
		path = filepath.Join(stateDir(), "team", time.Now().Format("20060102-150405")+"-"+slugify(topic)+".md")
	}
	path = expandHome(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
		if err := os.WriteFile(path, []byte(report), 0o644); err == nil {
			fmt.Fprintf(os.Stderr, "report: %s\n", path)
		} else {
			fmt.Fprintf(os.Stderr, "could not write report: %v\n", err)
		}
	}
	fmt.Print(report)
	if answered == 0 {
		return 1
	}
	return 0
}
