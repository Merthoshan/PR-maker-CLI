package command

import "context"

// Spec describes one external process invocation.
type Spec struct {
	Name        string
	Args        []string
	Dir         string
	Env         []string
	Stdin       string
	StdoutLimit int64
}

// Result contains the output produced by an external process.
type Result struct {
	Stdout          string
	Stderr          string
	StdoutTruncated bool
}

// Runner executes external commands.
type Runner interface {
	Run(ctx context.Context, spec Spec) (Result, error)
}

// StreamingRunner executes commands while delivering complete stdout lines as
// they arrive. The complete stream is still captured in Result, subject to the
// configured StdoutLimit.
type StreamingRunner interface {
	RunStreaming(ctx context.Context, spec Spec, onStdoutLine func(string)) (Result, error)
}
