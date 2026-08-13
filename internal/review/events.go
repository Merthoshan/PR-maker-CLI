package review

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
)

const maxCodexEventBytes = 16 << 20

type codexEventParser struct {
	lineNumber  int
	finalReview string
	usage       TokenUsage
	err         error
	onUsage     func(TokenUsage)
}

func (parser *codexEventParser) consume(line string) {
	parser.lineNumber++
	if parser.err != nil || strings.TrimSpace(line) == "" {
		return
	}
	var envelope eventEnvelope
	if err := json.Unmarshal([]byte(line), &envelope); err != nil {
		parser.err = fmt.Errorf("malformed Codex JSON event at line %d", parser.lineNumber)
		return
	}
	switch envelope.Type {
	case "item.completed":
		var event itemCompletedEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			parser.err = fmt.Errorf("malformed Codex item event at line %d", parser.lineNumber)
			return
		}
		if event.Item.Type == "agent_message" && strings.TrimSpace(event.Item.Text) != "" {
			parser.finalReview = event.Item.Text
		}
	case "turn.completed":
		var event turnCompletedEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			parser.err = fmt.Errorf("malformed Codex usage event at line %d", parser.lineNumber)
			return
		}
		usage, err := tokenUsageFromEvent(event.Usage)
		if err != nil {
			parser.err = fmt.Errorf("invalid Codex usage event at line %d", parser.lineNumber)
			return
		}
		parser.usage = usage
		if usage.Available && parser.onUsage != nil {
			parser.onUsage(usage)
		}
	case "error", "turn.failed":
		parser.err = errors.New("Codex reported that the review turn failed")
	}
}

func (parser *codexEventParser) consumeBuffered(output string) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 64<<10), maxCodexEventBytes)
	for scanner.Scan() {
		parser.consume(scanner.Text())
	}
	if err := scanner.Err(); err != nil && parser.err == nil {
		parser.err = errors.New("Codex emitted an event larger than the supported limit")
	}
}

func (parser *codexEventParser) result() (string, TokenUsage, error) {
	if parser.err != nil {
		return "", TokenUsage{}, parser.err
	}
	if strings.TrimSpace(parser.finalReview) == "" {
		return "", TokenUsage{}, errors.New("Codex event stream did not contain a final review")
	}
	return parser.finalReview, parser.usage, nil
}

func tokenUsageFromEvent(raw *rawTokenUsage) (TokenUsage, error) {
	if raw == nil || raw.InputTokens == nil || raw.OutputTokens == nil {
		return TokenUsage{}, nil
	}
	values := []*int64{
		raw.InputTokens,
		raw.CachedInputTokens,
		raw.CacheWriteInputTokens,
		raw.OutputTokens,
		raw.ReasoningOutputTokens,
	}
	for _, value := range values {
		if value != nil && *value < 0 {
			return TokenUsage{}, errors.New("token counts cannot be negative")
		}
	}
	if *raw.InputTokens > math.MaxInt64-*raw.OutputTokens {
		return TokenUsage{}, errors.New("token total overflows")
	}
	usage := TokenUsage{
		Available:    true,
		InputTokens:  *raw.InputTokens,
		OutputTokens: *raw.OutputTokens,
	}
	if raw.CachedInputTokens != nil {
		usage.CachedInputAvailable = true
		usage.CachedInputTokens = *raw.CachedInputTokens
	}
	if raw.CacheWriteInputTokens != nil {
		usage.CacheWriteInputAvailable = true
		usage.CacheWriteInputTokens = *raw.CacheWriteInputTokens
	}
	if raw.ReasoningOutputTokens != nil {
		usage.ReasoningOutputAvailable = true
		usage.ReasoningOutputTokens = *raw.ReasoningOutputTokens
	}
	return usage, nil
}
