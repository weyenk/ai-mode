package app

// laterStages are the stages from implementation on; defaultStages registers them with the rest.
var laterStages = map[string]stageFunc{
	"story:ready":            stageImplement,
	"story:architect-review": stageReview,
	"story:handoff":          stageHandoff,
}
