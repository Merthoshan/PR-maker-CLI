package version

import (
	"errors"
	"fmt"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
)

const Development = "development"

var releasePattern = regexp.MustCompile(
	`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`,
)

// Current returns the semantic module version embedded by Go installations.
func Current() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return Development
	}
	if !IsRelease(info.Main.Version) {
		return Development
	}
	return info.Main.Version
}

// IsRelease reports whether value is a strict vMAJOR.MINOR.PATCH version.
func IsRelease(value string) bool {
	return releasePattern.MatchString(strings.TrimSpace(value))
}

// Compare compares two strict semantic versions.
func Compare(left string, right string) (int, error) {
	leftRelease, err := parse(left)
	if err != nil {
		return 0, fmt.Errorf("compare left version: %w", err)
	}
	rightRelease, err := parse(right)
	if err != nil {
		return 0, fmt.Errorf("compare right version: %w", err)
	}

	leftParts := [...]uint64{
		leftRelease.major,
		leftRelease.minor,
		leftRelease.patch,
	}
	rightParts := [...]uint64{
		rightRelease.major,
		rightRelease.minor,
		rightRelease.patch,
	}
	for index := range leftParts {
		switch {
		case leftParts[index] < rightParts[index]:
			return -1, nil
		case leftParts[index] > rightParts[index]:
			return 1, nil
		}
	}
	return 0, nil
}

func parse(value string) (release, error) {
	value = strings.TrimSpace(value)
	matches := releasePattern.FindStringSubmatch(value)
	if len(matches) != 4 {
		return release{}, errors.New(
			"version must use the vMAJOR.MINOR.PATCH format",
		)
	}

	parts := make([]uint64, 3)
	for index, match := range matches[1:] {
		part, err := strconv.ParseUint(match, 10, 64)
		if err != nil {
			return release{}, fmt.Errorf("parse version component: %w", err)
		}
		parts[index] = part
	}
	return release{
		major: parts[0],
		minor: parts[1],
		patch: parts[2],
	}, nil
}
