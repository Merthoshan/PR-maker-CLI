package gitcontext

// Repository describes the Git repository and branch being analyzed.
type Repository struct {
	Root      string
	Branch    string
	HeadSHA   string
	RemoteURL string
	Dirty     bool
}

// Evidence contains the Git facts used to generate a pull-request description.
type Evidence struct {
	BaseBranch   string
	BaseRef      string
	MergeBaseSHA string
	CommitLog    string
	ChangedFiles string
	Diff         string
}
