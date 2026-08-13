package review

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Merthoshan/PR-maker-CLI/internal/github"
)

const (
	standardEvidenceTokenBudget = 32_000
	deepEvidenceTokenBudget     = 96_000
	standardBodyTokenBudget     = 4_000
	deepBodyTokenBudget         = 12_000
	minimumDiffTokenBudget      = 1_000
	estimatedBytesPerToken      = 2
)

func buildPayload(data github.ReviewData, depth string, instructions string) (payload, error) {
	pullRequest := data.PullRequest
	bodyBudget := standardBodyTokenBudget
	if depth == Deep {
		bodyBudget = deepBodyTokenBudget
	}
	var bodyLimited bool
	pullRequest.Body, bodyLimited = limitTextTokens(pullRequest.Body, bodyBudget)

	result := payload{
		PullRequest:            pullRequest,
		Labels:                 data.Labels,
		Repository:             data.Repository,
		ChangedFiles:           data.Files,
		SourceDiffLimited:      data.DiffLimited,
		PullRequestBodyLimited: bodyLimited,
		Depth:                  depth,
		ReviewInstructions:     instructions,
	}
	baseJSON, err := json.Marshal(result)
	if err != nil {
		return payload{}, fmt.Errorf("review: encode evidence metadata: %w", err)
	}
	totalBudget := evidenceTokenBudget(depth)
	diffBudget := totalBudget - estimatedTokens(string(baseJSON))
	if diffBudget < minimumDiffTokenBudget {
		return payload{}, fmt.Errorf(
			"review: metadata and custom instructions leave fewer than %d tokens for the diff",
			minimumDiffTokenBudget,
		)
	}

	for attempts := 0; attempts < 3; attempts++ {
		selection := selectDiff(data.Diff, data.Files, diffBudget, data.DiffLimited)
		result.Diff = selection.Diff
		result.OmittedFiles = selection.OmittedFiles
		result.EvidenceOmitted = selection.Omitted || bodyLimited || data.DiffLimited
		encoded, err := json.Marshal(result)
		if err != nil {
			return payload{}, fmt.Errorf("review: encode selected evidence: %w", err)
		}
		estimate := estimatedTokens(string(encoded))
		result.EvidenceTokenEstimate = estimate
		finalEncoded, err := json.Marshal(result)
		if err != nil {
			return payload{}, fmt.Errorf("review: encode token estimate: %w", err)
		}
		finalEstimate := estimatedTokens(string(finalEncoded))
		result.EvidenceTokenEstimate = finalEstimate
		if finalEstimate <= totalBudget {
			return result, nil
		}
		diffBudget -= finalEstimate - totalBudget + 256
		if diffBudget < minimumDiffTokenBudget {
			break
		}
	}
	return payload{}, errors.New("review: selected evidence exceeds the configured token budget")
}

func evidenceTokenBudget(depth string) int {
	if depth == Deep {
		return deepEvidenceTokenBudget
	}
	return standardEvidenceTokenBudget
}

// estimatedTokens intentionally overestimates typical source-code token use.
// Exact tokenization depends on the active model, so two UTF-8 bytes per
// token provides a stable local budget without coupling to one model encoder.
func estimatedTokens(value string) int {
	if value == "" {
		return 0
	}
	return (len(value) + estimatedBytesPerToken - 1) / estimatedBytesPerToken
}

func limitTextTokens(value string, tokenBudget int) (string, bool) {
	if estimatedTokens(value) <= tokenBudget {
		return value, false
	}
	byteBudget := tokenBudget * estimatedBytesPerToken
	if byteBudget >= len(value) {
		return value, false
	}
	cut := byteBudget
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return strings.TrimSpace(value[:cut]), true
}

func selectDiff(
	diff string,
	files []github.ChangedFile,
	tokenBudget int,
	sourceLimited bool,
) diffSelection {
	parts := parseFileDiffs(diff, sourceLimited)
	if len(parts) == 0 {
		return diffSelection{
			Omitted:      true,
			OmittedFiles: changedFilePaths(files),
		}
	}

	ranked := append([]fileDiff(nil), parts...)
	sort.SliceStable(ranked, func(left int, right int) bool {
		return ranked[left].score > ranked[right].score
	})
	selected := make([]selectedFileDiff, 0, len(ranked))
	remaining := tokenBudget
	for _, part := range ranked {
		cost := estimatedTokens(part.text)
		if part.complete && cost <= remaining {
			selected = append(selected, selectedFileDiff{
				index: part.index,
				text:  part.text,
				full:  true,
				path:  part.path,
			})
			remaining -= cost
			continue
		}
		partial := selectCompleteHunks(part.text, remaining, part.complete)
		if partial != "" {
			selected = append(selected, selectedFileDiff{
				index: part.index,
				text:  partial,
				path:  part.path,
			})
			remaining -= estimatedTokens(partial)
		}
	}

	sort.Slice(selected, func(left int, right int) bool {
		return selected[left].index < selected[right].index
	})
	var builder strings.Builder
	fullyIncluded := make(map[string]bool, len(selected))
	for _, part := range selected {
		builder.WriteString(strings.TrimSuffix(part.text, "\n"))
		builder.WriteByte('\n')
		if part.full {
			fullyIncluded[part.path] = true
		}
	}

	omittedFiles := make([]string, 0)
	seen := make(map[string]bool)
	for _, file := range files {
		if file.Path == "" || fullyIncluded[file.Path] || seen[file.Path] {
			continue
		}
		seen[file.Path] = true
		omittedFiles = append(omittedFiles, file.Path)
	}
	for _, part := range parts {
		if part.path == "" || fullyIncluded[part.path] || seen[part.path] {
			continue
		}
		seen[part.path] = true
		omittedFiles = append(omittedFiles, part.path)
	}
	return diffSelection{
		Diff:         builder.String(),
		Omitted:      sourceLimited || len(omittedFiles) > 0,
		OmittedFiles: omittedFiles,
	}
}

func parseFileDiffs(diff string, sourceLimited bool) []fileDiff {
	starts := make([]int, 0)
	if strings.HasPrefix(diff, "diff --git ") {
		starts = append(starts, 0)
	}
	for offset := 0; ; {
		index := strings.Index(diff[offset:], "\ndiff --git ")
		if index < 0 {
			break
		}
		start := offset + index + 1
		starts = append(starts, start)
		offset = start + len("diff --git ")
	}
	if len(starts) == 0 {
		return nil
	}

	parts := make([]fileDiff, 0, len(starts))
	for index, start := range starts {
		end := len(diff)
		if index+1 < len(starts) {
			end = starts[index+1]
		}
		text := diff[start:end]
		path := pathFromDiffHeader(text)
		complete := !sourceLimited || index != len(starts)-1
		parts = append(parts, fileDiff{
			index:    index,
			path:     path,
			text:     text,
			complete: complete,
			score:    riskScore(path, text),
		})
	}
	return parts
}

func pathFromDiffHeader(section string) string {
	line := section
	if newline := strings.IndexByte(line, '\n'); newline >= 0 {
		line = line[:newline]
	}
	marker := strings.LastIndex(line, " b/")
	if marker < 0 {
		return line
	}
	return strings.Trim(strings.TrimPrefix(line[marker+1:], "b/"), `"`)
}

func selectCompleteHunks(section string, tokenBudget int, sourceComplete bool) string {
	if tokenBudget <= 0 {
		return ""
	}
	firstHunk := strings.Index(section, "\n@@ ")
	if firstHunk < 0 {
		if sourceComplete && estimatedTokens(section) <= tokenBudget {
			return section
		}
		return ""
	}
	header := section[:firstHunk+1]
	if estimatedTokens(header) >= tokenBudget {
		return ""
	}
	hunkText := section[firstHunk+1:]
	starts := []int{0}
	for offset := 0; ; {
		index := strings.Index(hunkText[offset:], "\n@@ ")
		if index < 0 {
			break
		}
		start := offset + index + 1
		starts = append(starts, start)
		offset = start + len("@@ ")
	}
	hunkCount := len(starts)
	if !sourceComplete {
		hunkCount--
	}
	if hunkCount <= 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString(header)
	remaining := tokenBudget - estimatedTokens(header)
	included := 0
	for index := 0; index < hunkCount; index++ {
		start := starts[index]
		end := len(hunkText)
		if index+1 < len(starts) {
			end = starts[index+1]
		}
		hunk := hunkText[start:end]
		cost := estimatedTokens(hunk)
		if cost > remaining {
			continue
		}
		builder.WriteString(hunk)
		remaining -= cost
		included++
	}
	if included == 0 {
		return ""
	}
	return builder.String()
}

func riskScore(path string, section string) int {
	score := 0
	lowerPath := strings.ToLower(path)
	for _, term := range []string{"migration", "auth", "permission", "payment", "transaction", "security"} {
		if strings.Contains(lowerPath, term) {
			score += 3
		}
	}
	for line := range strings.Lines(section) {
		if !strings.HasPrefix(line, "+") || strings.HasPrefix(line, "+++") {
			continue
		}
		lower := strings.ToLower(line)
		for _, term := range []string{"sql", "query", "database", "migration", "auth", "permission", "token", "secret", "password", "payment", "transaction", "mutex", "goroutine", "api", "validation", "error"} {
			if strings.Contains(lower, term) {
				score += 2
			}
		}
		if strings.Contains(lower, "for ") || strings.Contains(lower, "range ") {
			score++
		}
	}
	if strings.HasSuffix(lowerPath, ".lock") || strings.HasSuffix(lowerPath, "go.sum") {
		score -= 4
	}
	return score
}

func changedFilePaths(files []github.ChangedFile) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		if file.Path != "" {
			paths = append(paths, file.Path)
		}
	}
	return paths
}

func omissionNotice(value payload) string {
	details := make([]string, 0, 3)
	if value.SourceDiffLimited {
		details = append(details, "GitHub diff capture reached the local memory limit")
	}
	if value.PullRequestBodyLimited {
		details = append(details, "the pull request body was shortened")
	}
	if len(value.OmittedFiles) > 0 {
		details = append(details, fmt.Sprintf(
			"%d file(s) were omitted or only partially included",
			len(value.OmittedFiles),
		))
	}
	if len(details) == 0 {
		details = append(details, "some evidence exceeded the configured review budget")
	}
	return "Evidence notice: " + strings.Join(details, "; ") + ". The omitted content was not reviewed."
}
