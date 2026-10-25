// Package releasetool exposes the release track rules to the release workflow.
// Results go to stdout as key=value lines or one value per line, errors go to
// stderr with a non-zero exit code.
package releasetool

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/m10rten/go-canary-cli/internal/release"
)

const usage = `usage:
  releasetool classify TAG                  validate a pushed tag
  releasetool next-nightly YYYYMMDD         release tags on stdin, one per line
  releasetool prune-nightlies KEEP          release tags on stdin, one per line
  releasetool dist-tags                     JSON from "npm view PKG versions dist-tags --json" on stdin`

type command struct {
	arity int
	run   func(args []string, stdin io.Reader) (string, error)
}

var commands = map[string]command{
	"classify":        {1, func(a []string, _ io.Reader) (string, error) { return classify(a[0]) }},
	"next-nightly":    {1, func(a []string, in io.Reader) (string, error) { return nextNightly(a[0], in) }},
	"prune-nightlies": {1, func(a []string, in io.Reader) (string, error) { return pruneNightlies(a[0], in) }},
	"dist-tags":       {0, func(_ []string, in io.Reader) (string, error) { return distTags(in) }},
}

// Run executes one command and returns the process exit code: 0 on success,
// 1 on a rejected input and 2 on a usage error. Callers that go through go run
// or make can rely only on zero versus non-zero.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	cmd, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(stderr, "releasetool: unknown command %q\n%s\n", args[0], usage)
		return 2
	}
	if len(args)-1 != cmd.arity {
		fmt.Fprintf(stderr, "releasetool: %s takes %d argument(s)\n%s\n", args[0], cmd.arity, usage)
		return 2
	}
	out, err := cmd.run(args[1:], stdin)
	if err != nil {
		fmt.Fprintf(stderr, "releasetool: %v\n", err)
		return 1
	}
	fmt.Fprint(stdout, out)
	return 0
}

func classify(tag string) (string, error) {
	track, err := release.ValidatePushedTag(tag)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("track=%s\nprerelease=%t\ndist_tag=%s\n", track, track.Prerelease(), track.DistTag()), nil
}

func nextNightly(date string, stdin io.Reader) (string, error) {
	day, err := time.Parse(release.NightlyDateLayout, date)
	if err != nil {
		return "", fmt.Errorf("date %q must look like YYYYMMDD", date)
	}
	tags, err := readLines(stdin)
	if err != nil {
		return "", err
	}
	plan, err := release.NextNightly(tags, day)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("tag=%s\nnewest=%s\nexists=%t\n", plan.Tag, plan.Newest, plan.Exists), nil
}

func pruneNightlies(keep string, stdin io.Reader) (string, error) {
	n, err := strconv.Atoi(keep)
	if err != nil {
		return "", fmt.Errorf("keep %q must be a number", keep)
	}
	tags, err := readLines(stdin)
	if err != nil {
		return "", err
	}
	doomed, err := release.NightlyDeletions(tags, n)
	if err != nil {
		return "", err
	}
	return joinLines(doomed), nil
}

func distTags(stdin io.Reader) (string, error) {
	var view struct {
		Versions json.RawMessage   `json:"versions"`
		DistTags map[string]string `json:"dist-tags"`
	}
	if err := json.NewDecoder(stdin).Decode(&view); err != nil {
		return "", fmt.Errorf("read npm view output: %w", err)
	}
	versions, err := parseVersions(view.Versions)
	if err != nil {
		return "", err
	}
	var lines []string
	for _, change := range release.DistTagChanges(versions, view.DistTags) {
		lines = append(lines, change.Tag+" "+change.Version)
	}
	return joinLines(lines), nil
}

// npm prints a single published version as a string, not a list.
func parseVersions(raw json.RawMessage) ([]string, error) {
	if string(raw) == "null" {
		return nil, fmt.Errorf("npm view output has null versions")
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return list, nil
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return []string{single}, nil
	}
	return nil, fmt.Errorf("npm view output has no versions list")
}

func readLines(r io.Reader) ([]string, error) {
	var lines []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}
