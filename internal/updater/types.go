package updater

type progressReporter interface {
	Start(string) func()
}

// Outcome describes the result of one explicit update request.
type Outcome struct {
	CurrentVersion string
	LatestVersion  string
	Updated        bool
	Cancelled      bool
}
