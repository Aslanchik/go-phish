package hypothesis

type Hypothesis struct {
	Brand          string `json:"brand"`
	TargetedAction string `json:"targeted_action"`
	Confidence     string `json:"confidence"`
	Reasoning      string `json:"reasoning"`
}
