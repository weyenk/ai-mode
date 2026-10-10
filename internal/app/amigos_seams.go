package app

// roleAsker abstracts the interaction with the LLM.
type roleAsker interface {
	Ask(role, user string, maxTokens int) (string, error)
}

// uncertaintyEngine calculates the agreement ratio for a question.
type uncertaintyEngine interface {
	Score(question string, answers []string) float64
}

// auditSink handles the persistence of meeting artifacts.
type auditSink interface {
	Write(kind string, v any) error
	Level() string
}

// layerClassifier determines the test level for a given task.
type layerClassifier interface {
	TestLayer(task string) (string, error)
}

// meetingOptions configures the execution of the meeting loop.
type meetingOptions struct {
	Rounds    int
	Audit     string
	ResumeID  string
	MeetingID string // directory name under amigos/; a fresh id, or ResumeID when resuming
	JSON      bool
	Resume    *Snapshot // set by the wiring when ResumeID names a saved meeting
}

// meetingRunner is the primary entrypoint for the meeting logic.
type meetingRunner func(in Intake, opts meetingOptions) (StoryContract, error)

// turnCodec handles the rendering of prompts and parsing of role replies.
type turnCodec interface {
	Prompt(role string, in Intake, c StoryContract, focus string) string
	Parse(role, raw string) (Turn, error)
}
