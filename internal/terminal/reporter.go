package terminal

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const (
	defaultInterval = 100 * time.Millisecond
	clearLine       = "\r\x1b[2K"
)

// Reporter displays progress for long-running terminal operations.
type Reporter struct {
	output      io.Writer
	interactive bool
	interval    time.Duration
	now         func() time.Time
}

// NewReporter creates a reporter that animates only when output is a terminal.
func NewReporter(output io.Writer) (*Reporter, error) {
	if output == nil {
		return nil, errors.New("create terminal reporter: output is required")
	}
	return newReporter(output, isTerminal(output), defaultInterval, time.Now), nil
}

// Start begins reporting one operation and returns an idempotent stop function.
func (reporter *Reporter) Start(message string) func() {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "Working"
	}
	if !reporter.interactive {
		fmt.Fprintf(reporter.output, "%s...\n", message)
		return func() {}
	}

	startedAt := reporter.now()
	reporter.render(spinnerFrames[0], message, startedAt)

	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(reporter.interval)
		defer ticker.Stop()

		frame := 1
		for {
			select {
			case <-ticker.C:
				reporter.render(
					spinnerFrames[frame%len(spinnerFrames)],
					message,
					startedAt,
				)
				frame++
			case <-done:
				fmt.Fprint(reporter.output, clearLine)
				return
			}
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			<-stopped
		})
	}
}

func newReporter(
	output io.Writer,
	interactive bool,
	interval time.Duration,
	now func() time.Time,
) *Reporter {
	return &Reporter{
		output:      output,
		interactive: interactive,
		interval:    interval,
		now:         now,
	}
}

func (reporter *Reporter) render(
	frame string,
	message string,
	startedAt time.Time,
) {
	elapsed := reporter.now().Sub(startedAt).Truncate(time.Second)
	if elapsed >= time.Second {
		fmt.Fprintf(
			reporter.output,
			"%s%s %s... %s",
			clearLine,
			frame,
			message,
			elapsed,
		)
		return
	}
	fmt.Fprintf(reporter.output, "%s%s %s...", clearLine, frame, message)
}

func isTerminal(output io.Writer) bool {
	file, ok := output.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
