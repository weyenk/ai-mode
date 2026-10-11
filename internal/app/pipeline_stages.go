package app

import "errors"

var errStageNotBuilt = errors.New("stage not built yet")

func stageReview(pl *pipeline, c *card) (string, cardNote, error) {
	return "", cardNote{}, errStageNotBuilt
}
func stageHandoff(pl *pipeline, c *card) (string, cardNote, error) {
	return "", cardNote{}, errStageNotBuilt
}

// laterStages are registered as they are built; a card whose stage has no runner waits.
var laterStages = map[string]stageFunc{"story:ready": stageImplement}
