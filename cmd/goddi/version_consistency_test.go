package main

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The release procedure has to update the version in more than one place, and
// the failure mode of missing one is silent: the binary reports the new number
// while a container built from the compose file reports the old one, and a
// support conversation starts from two different versions.
//
// This test pins the places that actually take effect at runtime. It is not a
// substitute for the release checklist; it is what stops the checklist being
// the only thing holding the two together.
func TestTheVersionIsTheSameEverywhereItTakesEffect(t *testing.T) {
	composePath := filepath.Join("..", "..", "docker-compose.yml")
	raw, err := os.ReadFile(composePath)
	if errors.Is(err, os.ErrNotExist) {
		t.Skipf("%s is not in this tree", composePath)
	}
	if err != nil {
		t.Fatalf("read %s: %v", composePath, err)
	}

	match := regexp.MustCompile(`(?m)^\s*VERSION:\s*"([^"]*)"`).FindStringSubmatch(string(raw))
	if match == nil {
		t.Fatalf("no VERSION build argument found in %s", composePath)
	}
	if match[1] != Version {
		t.Fatalf("%s builds with VERSION %q while the binary reports %q; bump both or neither",
			composePath, match[1], Version)
	}
}

// A Dockerfile default that is a version number is a version number that
// drifts. The other two callers always pass one, so the default only ever
// shows up in a local build -- where a wrong value is worse than an obviously
// provisional one.
func TestTheDockerfileDoesNotDefaultToAVersionNumber(t *testing.T) {
	dockerfilePath := filepath.Join("..", "..", "Dockerfile")
	raw, err := os.ReadFile(dockerfilePath)
	if errors.Is(err, os.ErrNotExist) {
		t.Skipf("%s is not in this tree", dockerfilePath)
	}
	if err != nil {
		t.Fatalf("read %s: %v", dockerfilePath, err)
	}

	match := regexp.MustCompile(`(?m)^ARG VERSION=(.*)$`).FindStringSubmatch(string(raw))
	if match == nil {
		t.Fatalf("no VERSION build argument found in %s", dockerfilePath)
	}
	value := strings.TrimSpace(match[1])
	if regexp.MustCompile(`^\d+\.\d+`).MatchString(value) {
		t.Fatalf("%s defaults VERSION to %q; a version number here drifts out of date without anything noticing",
			dockerfilePath, value)
	}
}
