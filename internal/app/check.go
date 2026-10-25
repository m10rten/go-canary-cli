package app

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/m10rten/go-canary-cli/internal/cli"
	"github.com/m10rten/go-canary-cli/internal/release"
)

type checkResult struct {
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	UpdateAvailable bool   `json:"update_available"`
	Track           string `json:"track"`
}

// updateAvailable reports whether latest is newer than current. A build that
// does not parse as a release (such as dev) is older than any release.
func updateAvailable(current, latest string) bool {
	return release.Compare(current, latest) < 0
}

// latestRelease returns the newest release a build of current should have:
// releases/latest for stable, otherwise the highest release in the upgrade path
// of its track. It returns current when the path holds no release.
func latestRelease(ctx context.Context, client release.Client, current string) (string, error) {
	track := release.TrackOf(current)
	if track == release.Stable {
		return client.LatestTag(ctx)
	}
	tags, err := client.ReleaseTags(ctx)
	if err != nil {
		return "", err
	}
	if tag, ok := release.HighestInPath(track, tags); ok && release.Compare(tag, current) >= 0 {
		return tag, nil
	}
	return current, nil
}

// check exits 0 when up to date and 1 when an update is available.
func (a *App) check(ctx *cli.Context) error {
	client := a.GitHub
	if token, ok := a.lookupEnv("GITHUB_TOKEN"); ok {
		client.Token = token
	}

	task := a.reporter(ctx).Spin("Checking for updates")
	latest, err := latestRelease(context.Background(), client, a.Build.Version)
	if err != nil {
		task.Fail("Could not check for updates")
		return fmt.Errorf("look up latest release: %w", err)
	}
	task.Done("Latest release is " + latest)

	got := checkResult{Current: a.Build.Version, Latest: latest, UpdateAvailable: updateAvailable(a.Build.Version, latest), Track: string(release.TrackOf(a.Build.Version))}
	if err := a.printCheck(ctx, got); err != nil {
		return err
	}
	if got.UpdateAvailable {
		return &cli.ExitError{Code: 1}
	}
	return nil
}

func (a *App) printCheck(ctx *cli.Context, got checkResult) error {
	track := ""
	if got.Track != string(release.Stable) {
		track = "track: " + got.Track
	}
	var err error
	switch {
	case ctx.Bool("json"):
		err = json.NewEncoder(a.Stdout).Encode(got)
	case got.UpdateAvailable && track != "":
		_, err = fmt.Fprintf(a.Stdout, "Update available: %s → %s (%s)\n", got.Current, got.Latest, track)
	case got.UpdateAvailable:
		_, err = fmt.Fprintf(a.Stdout, "Update available: %s → %s\n", got.Current, got.Latest)
	case track != "":
		_, err = fmt.Fprintf(a.Stdout, "Up to date (%s, %s)\n", got.Current, track)
	default:
		_, err = fmt.Fprintf(a.Stdout, "Up to date (%s)\n", got.Current)
	}
	return err
}
