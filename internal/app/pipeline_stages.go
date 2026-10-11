package app

import "errors"

var errStageNotBuilt = errors.New("stage not built yet")

func stageAmigos(pl *pipeline, c *card) (string, cardNote, error) {
	return "", cardNote{}, errStageNotBuilt
}
func stageImplement(pl *pipeline, c *card) (string, cardNote, error) {
	return "", cardNote{}, errStageNotBuilt
}
func stageReview(pl *pipeline, c *card) (string, cardNote, error) {
	return "", cardNote{}, errStageNotBuilt
}
func stageHandoff(pl *pipeline, c *card) (string, cardNote, error) {
	return "", cardNote{}, errStageNotBuilt
}
