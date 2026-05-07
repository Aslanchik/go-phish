package db

type Status string

const (
	StatusPending       Status = "pending"
	StatusFetching      Status = "fetching"
	StatusHypothesizing Status = "hypothesizing"
	StatusEnriching     Status = "enriching"
	StatusComplete      Status = "complete"
	StatusFailed        Status = "failed"
)

func (s Status) String() string { return string(s) }
