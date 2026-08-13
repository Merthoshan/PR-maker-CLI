package cli

// Options contains the command-line choices that affect one champu-pr run.
type Options struct {
	Branch             bool
	BranchCleanup      bool
	Review             bool
	ReviewTarget       string
	ReviewDepth        string
	ReviewInstructions string
	Base               string
	PRNumber           int
	Ready              bool
	DryRun             bool
}
