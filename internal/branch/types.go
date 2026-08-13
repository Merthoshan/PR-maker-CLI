package branch

import "time"

// Request describes one branch command invocation.
type Request struct {
	WorkingDirectory string
	Cleanup          bool
}

type localBranch struct {
	Name       string
	CommitTime time.Time
	CommitDate string
	Current    bool
	Merged     bool
}

type snapshot struct {
	branches []localBranch
	current  string
	base     string
}

type skippedBranch struct {
	name   string
	reason string
}
