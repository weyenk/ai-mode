package app

import (
	"os"
	"path/filepath"
)

type wireDeps struct {
	Asker         roleAsker
	Codec         turnCodec
	Engine        uncertaintyEngine
	Classifier    layerClassifier
	Callers       []string
	Roots         []string
	StateDir      string
	MaxAuditBytes int64
}

func newMeetingRunnerWith(d wireDeps) meetingRunner {
	return func(in Intake, opts meetingOptions) (StoryContract, error) {
		if opts.ResumeID != "" {
			snap, err := loadSnapshot(filepath.Join(d.StateDir, "amigos"), opts.ResumeID)
			if err != nil {
				return StoryContract{}, err
			}
			opts.Resume = &snap
			if in.Story == "" {
				in.Story = snap.Contract.Story
			}
		}
		if e := validateIntake(in, d.Roots, d.Callers); e != nil {
			return StoryContract{}, e
		}
		sink, err := newFileAudit(opts.Audit, filepath.Join(d.StateDir, "amigos", opts.MeetingID), d.MaxAuditBytes)
		if err != nil {
			return StoryContract{}, err
		}
		c, err := runMeeting(in, opts, d.Asker, d.Codec, d.Engine, sink, d.Classifier)
		if err != nil {
			return c, err
		}
		c.Gates = planGates(in.Constraints.Exposed)
		if verr := validateLayerPlan(c); verr != nil {
			if c.Verdict == VerdictReady {
				c.Verdict = VerdictNotReady
				c.Questions = append(c.Questions, Question{Text: "layer plan: " + verr.Error(), State: QuestionOpen})
			}
		} else if err := assignTestLayers(&c, d.Classifier); err != nil {
			return c, err
		}
		return c, saveFinal(sink, d.StateDir, opts.MeetingID, c)
	}
}

func newMeetingRunner() meetingRunner {
	fail := func(err error) meetingRunner {
		return func(Intake, meetingOptions) (StoryContract, error) { return StoryContract{}, err }
	}
	p, code := activeProfile("", "Use: ai-mode use <name>")
	if code != 0 {
		return fail(os.ErrNotExist)
	}
	callers, err := loadCallers(filepath.Join(presetsDir(), "amigos-callers.list"))
	if err != nil {
		return fail(err)
	}
	cwd, _ := os.Getwd()
	return newMeetingRunnerWith(wireDeps{
		Asker: newRoleAsker(p, "amigos", runAsk), Codec: jsonCodec{}, Engine: agreementEngine{},
		Classifier: newJevClassifier(p, classifyCall), Callers: callers, Roots: []string{cwd},
		StateDir: stateDir(), MaxAuditBytes: 50 << 20,
	})
}

func cmdAmigos(args []string) int {
	return runAmigos(args, newMeetingRunner(), os.Stdin, os.Stdout, os.Stderr)
}

// saveFinal rewrites the saved contract with the post-loop result (gates, test levels, verdict),
// keeping the meeting state the loop recorded. A level that keeps no contract has nothing to rewrite.
func saveFinal(sink auditSink, stateDir, id string, c StoryContract) error {
	if sink.Level() == "off" {
		return nil
	}
	snap, err := loadSnapshot(filepath.Join(stateDir, "amigos"), id)
	if err != nil {
		return err
	}
	snap.Contract = c
	return sink.Write("contract", snap)
}
