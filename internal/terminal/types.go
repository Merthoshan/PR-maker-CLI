package terminal

// Status describes one stage in a multi-stage operation. Percent is an
// approximate workflow percentage, not a token-usage percentage.
type Status struct {
	Message string
	Percent int
	Details []string
}
