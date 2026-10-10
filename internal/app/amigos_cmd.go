package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

func exitCodeFor(v Verdict) int {
	switch v {
	case VerdictReady:
		return ExitReady
	case VerdictNotReady:
		return ExitNotReady
	case VerdictPaused:
		return ExitPaused
	}
	return ExitError
}

func newMeetingID() string {
	b := make([]byte, 3)
	rand.Read(b)
	return time.Now().UTC().Format("20060102-150405") + "-" + hex.EncodeToString(b)
}

func runAmigos(args []string, run meetingRunner, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := newFlags("amigos")
	fs.SetOutput(io.Discard)
	story := fs.String("story", "", "story file, or - for stdin")
	rounds := fs.Int("rounds", 2, "rounds")
	audit := fs.String("audit", "summary", "off|summary|full")
	resume := fs.String("resume", "", "meeting id to resume")
	asJSON := fs.Bool("json", false, "JSON output")
	out := fs.String("out", "", "also write the output here")
	caller := fs.String("caller", "human", "caller id")
	usage := func() int {
		fmt.Fprintln(stderr, "usage: ai-mode amigos <story...> [--story FILE|-] [--rounds N] [--audit off|summary|full] [--resume ID] [--json] [--out FILE] [--caller C]")
		return ExitError
	}
	pos, _, ok := parse(fs, args)
	if !ok || *rounds < 2 || (*audit != "off" && *audit != "summary" && *audit != "full") {
		return usage()
	}
	text := strings.TrimSpace(strings.Join(pos, " "))
	if text == "" && *story == "-" {
		b, _ := io.ReadAll(stdin)
		text = strings.TrimSpace(string(b))
	} else if text == "" && *story != "" {
		b, err := os.ReadFile(*story)
		if err != nil {
			fmt.Fprintln(stderr, "error: "+err.Error())
			return ExitError
		}
		text = strings.TrimSpace(string(b))
	}
	if text == "" && *resume == "" {
		return usage()
	}
	in := Intake{Story: text, Provenance: Provenance{Kind: "human", CallerID: *caller}}
	if strings.HasPrefix(text, "{") {
		var c Intake
		if json.Unmarshal([]byte(text), &c) == nil && c.Story != "" {
			in = c
			if *caller != "human" || in.Provenance.CallerID == "" {
				in.Provenance.CallerID = *caller
			}
		}
	}
	id := *resume
	if id == "" {
		id = newMeetingID()
	}
	c, err := run(in, meetingOptions{Rounds: *rounds, Audit: *audit, ResumeID: *resume, MeetingID: id, JSON: *asJSON})
	if err != nil {
		var ie *IntakeError
		if errors.As(err, &ie) {
			b, _ := json.Marshal(ie)
			fmt.Fprintln(stderr, string(b))
		} else {
			fmt.Fprintln(stderr, "error: "+err.Error())
		}
		return ExitError
	}
	var sb strings.Builder
	if *asJSON {
		b, _ := json.MarshalIndent(c, "", "  ")
		sb.Write(b)
		sb.WriteString("\n")
	} else {
		fmt.Fprintf(&sb, "verdict: %s\nlayer plan:\n", c.Verdict)
		for _, l := range c.LayerPlan {
			var ps []string
			for _, p := range l.Scenarios {
				ps = append(ps, fmt.Sprint(p))
			}
			fmt.Fprintf(&sb, "  %d %s [%s] examples: %s\n", l.N, l.Name, l.TestLevel, strings.Join(ps, ","))
		}
		sb.WriteString("questions:\n")
		for _, q := range c.Questions {
			if q.State != QuestionResolved {
				fmt.Fprintf(&sb, "  [%s] %s\n", q.State, q.Text)
			}
		}
		sb.WriteString("contract:\n  rules:\n")
		for _, r := range c.Rules {
			fmt.Fprintf(&sb, "    - %s\n", r)
		}
		sb.WriteString("  examples:\n")
		for _, e := range c.Examples {
			fmt.Fprintf(&sb, "    - (%s) %s\n", e.Rule, e.Text)
		}
		if c.Verdict == VerdictPaused {
			fmt.Fprintf(&sb, "resume with: ai-mode amigos --resume %s\n", id)
		}
	}
	io.WriteString(stdout, sb.String())
	if *out != "" {
		os.WriteFile(*out, []byte(sb.String()), 0o600)
	}
	return exitCodeFor(c.Verdict)
}
