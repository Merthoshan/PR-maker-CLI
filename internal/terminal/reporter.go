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

// StartDetailed begins one multi-stage operation and returns update and stop
// functions. Interactive output is animated in place; redirected output uses
// deterministic static lines.
func (reporter *Reporter) StartDetailed(initial Status) (func(Status), func()) {
	initial = normalizeStatus(initial)
	if !reporter.interactive {
		var mutex sync.Mutex
		var last Status
		var wrote bool
		write := func(status Status) {
			mutex.Lock()
			defer mutex.Unlock()
			status = normalizeStatus(status)
			if wrote && status.Message == last.Message && status.Percent == last.Percent {
				return
			}
			reporter.renderStaticStatus(status)
			last = status
			wrote = true
		}
		write(initial)
		return write, func() {}
	}

	startedAt := reporter.now()
	current := initial
	var statusMutex sync.RWMutex
	reporter.renderDetailed(spinnerFrames[0], current, startedAt)

	updated := make(chan struct{}, 1)
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
			case <-updated:
			case <-done:
				fmt.Fprint(reporter.output, clearLine)
				return
			}
			statusMutex.RLock()
			status := current
			statusMutex.RUnlock()
			reporter.renderDetailed(
				spinnerFrames[frame%len(spinnerFrames)],
				status,
				startedAt,
			)
			frame++
		}
	}()

	update := func(status Status) {
		statusMutex.Lock()
		current = normalizeStatus(status)
		statusMutex.Unlock()
		select {
		case updated <- struct{}{}:
		default:
		}
	}
	var once sync.Once
	stop := func() {
		once.Do(func() {
			close(done)
			<-stopped
		})
	}
	return update, stop
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

func (reporter *Reporter) renderDetailed(
	frame string,
	status Status,
	startedAt time.Time,
) {
	elapsed := reporter.now().Sub(startedAt).Truncate(time.Second)
	details := ""
	if len(status.Details) > 0 {
		details = " | " + strings.Join(status.Details, " | ")
	}
	fmt.Fprintf(
		reporter.output,
		"%s%s [%s] %3d%% %s | Elapsed: %s%s",
		clearLine,
		frame,
		progressBar(status.Percent),
		status.Percent,
		status.Message,
		formatElapsed(elapsed),
		details,
	)
}

func (reporter *Reporter) renderStaticStatus(status Status) {
	fmt.Fprintf(reporter.output, "[%3d%%] %s...\n", status.Percent, status.Message)
	for _, detail := range status.Details {
		fmt.Fprintf(reporter.output, "  %s\n", detail)
	}
}

func normalizeStatus(status Status) Status {
	status.Message = strings.TrimSpace(status.Message)
	if status.Message == "" {
		status.Message = "Working"
	}
	if status.Percent < 0 {
		status.Percent = 0
	}
	if status.Percent > 100 {
		status.Percent = 100
	}
	details := make([]string, 0, len(status.Details))
	for _, detail := range status.Details {
		detail = strings.Join(strings.Fields(detail), " ")
		if detail != "" {
			details = append(details, detail)
		}
	}
	status.Details = details
	return status
}

func progressBar(percent int) string {
	const width = 10
	filled := (percent*width + 99) / 100
	if percent == 0 {
		filled = 0
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func formatElapsed(elapsed time.Duration) string {
	seconds := int64(elapsed / time.Second)
	if seconds < 0 {
		seconds = 0
	}
	minutes := seconds / 60
	seconds %= 60
	if minutes < 60 {
		return fmt.Sprintf("%02d:%02d", minutes, seconds)
	}
	hours := minutes / 60
	minutes %= 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

func isTerminal(output io.Writer) bool {
	file, ok := output.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
