package github

// PullRequest contains the GitHub fields needed to select and update a PR.
type PullRequest struct {
	Number     int    `json:"number"`
	State      string `json:"state"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	Body       string `json:"body"`
	BaseBranch string `json:"baseRefName"`
	HeadBranch string `json:"headRefName"`
	Draft      bool   `json:"isDraft"`
}

// ReviewData contains the read-only GitHub data used by a PR review.
type ReviewData struct {
	PullRequest PullRequest
	Labels      []string
	Repository  string
	Files       []ChangedFile
	Diff        string
	DiffLimited bool
}

// ChangedFile summarizes one file in a pull request independently of whether
// its complete patch fits in the Codex evidence budget.
type ChangedFile struct {
	Path      string `json:"path"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

// ReviewRequest describes a bounded, repository-specific PR evidence lookup.
type ReviewRequest struct {
	RepositoryRoot     string
	Target             string
	ExpectedRepository string
	DiffByteLimit      int64
	BeforeDiff         func()
}

type reviewView struct {
	PullRequest
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Files []ChangedFile `json:"files"`
}

// PublishRequest describes one approved GitHub mutation.
type PublishRequest struct {
	RepositoryRoot string
	HeadBranch     string
	BaseBranch     string
	Title          string
	Body           string
	PullRequest    *PullRequest
	Ready          bool
}

// PublishResult reports the PR affected by publishing.
type PublishResult struct {
	URL     string
	Created bool
}

// Selection describes whether the workflow should update an existing pull
// request or create a new one.
type Selection struct {
	PullRequest  *PullRequest
	ShouldCreate bool
}
