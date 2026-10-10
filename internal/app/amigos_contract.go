package app

// Verdict is the outcome of an amigos meeting.
type Verdict string

const (
	VerdictReady    Verdict = "ready"
	VerdictNotReady Verdict = "not-ready"
	VerdictPaused   Verdict = "paused"
)

// Provenance says who submitted a story.
type Provenance struct {
	Kind     string `json:"kind"` // human | skill | agent
	CallerID string `json:"caller_id"`
}

// Constraints are operational limits that travel with a story.
type Constraints struct {
	Exposed bool `json:"exposed"` // the change touches an exposed surface
}

// Intake is the request contract for `ai-mode amigos`.
type Intake struct {
	Story       string      `json:"story"`
	Provenance  Provenance  `json:"provenance"`
	Repo        string      `json:"repo,omitempty"`
	Constraints Constraints `json:"constraints"`
}

// Codes returned by the intake gate.
const (
	ErrUnauthorizedCaller = "unauthorized_caller"
	ErrSchemaMismatch     = "schema_mismatch"
	ErrPathOutsideRoots   = "path_outside_roots"
	ErrSecretDetected     = "secret_detected"
)

// IntakeError is the structured error the intake gate returns.
type IntakeError struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

func (e *IntakeError) Error() string { return e.Code + ": " + e.Detail }

// Question is a point the meeting has to settle.
type Question struct {
	Text    string   `json:"text"`
	State   string   `json:"state"`
	Score   float64  `json:"score"`
	Answers []string `json:"answers,omitempty"`
}

// Question states.
const (
	QuestionOpen      = "open"
	QuestionParked    = "parked"
	QuestionReasked   = "re-asked"
	QuestionResolved  = "resolved"
	QuestionEscalated = "escalated"
)

// Example is a concrete instance of a rule.
type Example struct {
	Rule string `json:"rule"`
	Text string `json:"text"`
}

// Layer is one layer of the stacked-PR delivery plan.
type Layer struct {
	N         int    `json:"n"`
	Name      string `json:"name"`
	Scenarios []int  `json:"scenarios"` // 1-based positions in StoryContract.Examples
	TestLevel string `json:"test_level"`
}

// Gate is a CI check applied across the stack; State is active | not-available | not-applicable.
type Gate struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

// Disagreement records a conflict between roles about an item.
type Disagreement struct {
	Item      string   `json:"item"`
	Roles     []string `json:"roles"`
	Positions []string `json:"positions"`
}

// StoryContract is the agreement the meeting produces.
type StoryContract struct {
	Story         string         `json:"story"`
	Rules         []string       `json:"rules"`
	Examples      []Example      `json:"examples"`
	Questions     []Question     `json:"questions"`
	Size          int            `json:"size"`
	SplitInto     []string       `json:"split_into"`
	LayerPlan     []Layer        `json:"layer_plan"`
	Gates         []Gate         `json:"gates"`
	Verdict       Verdict        `json:"verdict"`
	Disagreements []Disagreement `json:"disagreements"`
}

// DecisionEntry is one line of the decision log.
type DecisionEntry struct {
	TS        string `json:"ts"`
	Round     int    `json:"round"`
	Role      string `json:"role"`
	Decision  string `json:"decision"`
	Rationale string `json:"rationale"`
}

// MeetingState is where a meeting stands between rounds.
type MeetingState struct {
	MeetingID string     `json:"meeting_id"`
	Round     int        `json:"round"`
	NextRole  string     `json:"next_role"`
	Parked    []Question `json:"parked"`
}

// Snapshot is everything needed to resume a meeting.
type Snapshot struct {
	Contract StoryContract `json:"contract"`
	State    MeetingState  `json:"state"`
}

// Answer is a role's answer to an earlier question.
type Answer struct {
	Question string `json:"question"`
	Text     string `json:"text"`
}

// Turn is one role's validated reply for one round (the role turn protocol).
type Turn struct {
	Role          string         `json:"role,omitempty"`
	Rules         []string       `json:"rules,omitempty"`
	Examples      []Example      `json:"examples,omitempty"`
	Questions     []string       `json:"questions,omitempty"`
	Answers       []Answer       `json:"answers,omitempty"`
	Size          int            `json:"size,omitempty"`
	SplitInto     []string       `json:"split_into,omitempty"`
	LayerPlan     []Layer        `json:"layer_plan,omitempty"`
	Disagreements []Disagreement `json:"disagreements,omitempty"`
	Ready         bool           `json:"ready"`
}

// Exit codes of `ai-mode amigos`.
const (
	ExitReady    = 0
	ExitError    = 1
	ExitNotReady = 2
	ExitPaused   = 3
)
