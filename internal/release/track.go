package release

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// Track is the release type of a version, inferred from the version string.
type Track string

const (
	Stable  Track = "stable"
	Nightly Track = "nightly"
	Alpha   Track = "alpha"
	Beta    Track = "beta"
	RC      Track = "rc"
)

// NightlyDateLayout is the UTC date format in a nightly tag.
const NightlyDateLayout = "20060102"

var upgradePaths = map[Track][]Track{
	Stable:  {Stable},
	Nightly: {Nightly, Stable},
	RC:      {RC, Stable},
	Beta:    {Beta, RC, Stable},
	Alpha:   {Alpha, Beta, RC, Stable},
}

// distTagTracks are the npm dist-tags the reconcile moves. "latest" is set by
// npm publish and never changed here.
var distTagTracks = []Track{Nightly, Alpha, Beta, RC}

// DistTagChange sets the npm dist-tag Tag to Version.
type DistTagChange struct {
	Tag     string
	Version string
}

// NightlyPlan is the result of deriving the next nightly tag.
type NightlyPlan struct {
	Tag    string // the nightly tag for the given day
	Newest string // the newest existing nightly tag, empty if none
	Exists bool   // Tag is already the newest existing nightly
}

// TrackOf infers the track of a version. A version with no prerelease part is
// stable. Otherwise the first prerelease identifier names the track. Anything
// else (v0.0.0-ci, a git describe string, "dev") is stable.
func TrackOf(version string) Track {
	v, ok := ParseVersion(version)
	if !ok {
		return Stable
	}
	if t, ok := classify(v); ok {
		return t
	}
	return Stable
}

// Path returns the tracks a user on t can upgrade to, including t itself.
func (t Track) Path() []Track {
	if path, ok := upgradePaths[t]; ok {
		return slices.Clone(path)
	}
	return []Track{Stable}
}

// DistTag returns the npm dist-tag that releases of t are published under.
func (t Track) DistTag() string {
	if t == Stable {
		return "latest"
	}
	return string(t)
}

// Prerelease reports whether releases of t are published as GitHub prereleases.
func (t Track) Prerelease() bool {
	return t != Stable
}

// HighestInPath returns the highest release in the upgrade path of track. It is
// what a fresh install on that track resolves to. Tags that do not parse or
// belong to no track are ignored.
func HighestInPath(track Track, releases []string) (string, bool) {
	return highestTag(releases, inTracks(track.Path()))
}

// SelectTarget returns the highest release in the upgrade path of current's
// track, if it is higher than current.
func SelectTarget(current string, releases []string) (string, bool) {
	target, ok := HighestInPath(TrackOf(current), releases)
	if !ok || Compare(target, current) <= 0 {
		return "", false
	}
	return target, true
}

// ValidatePushedTag accepts the tags a person pushes to release: vX.Y.Z and
// vX.Y.Z-(alpha|beta|rc).N with N >= 1. Nightly tags are created by the
// workflow, so a pushed one is rejected.
func ValidatePushedTag(tag string) (Track, error) {
	if !strings.HasPrefix(tag, "v") {
		return "", fmt.Errorf("tag %q must start with v", tag)
	}
	v, ok := ParseVersion(tag)
	if !ok {
		return "", fmt.Errorf("tag %q is not a valid semantic version such as v1.2.3", tag)
	}
	if strings.Contains(tag, "+") {
		return "", fmt.Errorf("tag %q must not have build metadata", tag)
	}
	if len(v.Pre) == 0 {
		return Stable, nil
	}
	track, known := classify(v)
	switch {
	case track == Nightly:
		return "", fmt.Errorf("tag %q is a nightly tag: nightlies are created by the workflow, not pushed", tag)
	case !known:
		return "", fmt.Errorf("tag %q has an unknown prerelease: want -alpha.N, -beta.N or -rc.N", tag)
	case len(v.Pre) != 2 || !isNumeric(v.Pre[1]) || v.Pre[1] == "0":
		return "", fmt.Errorf("tag %q must look like v1.2.3-%s.N with N of 1 or more", tag, track)
	}
	return track, nil
}

// NextNightly derives the nightly tag for the UTC day of now from the existing
// release tags. The base is the highest stable tag with its patch raised by
// one (v0.0.0 if there is none), so the nightly sorts above the last stable and
// below any next stable. It fails if the tag would sort below an existing nightly.
// If a nightly for that UTC day already exists, it is returned with Exists set so
// a day never gets a second nightly. tags must be every existing tag, including
// unpublished ones, so Exists is reliable.
func NextNightly(tags []string, now time.Time) (NightlyPlan, error) {
	var base Version
	if stable, ok := highestTag(tags, func(v Version) bool { return len(v.Pre) == 0 }); ok {
		base, _ = ParseVersion(stable)
	}
	date := now.UTC().Format(NightlyDateLayout)
	plan := NightlyPlan{
		Tag: fmt.Sprintf("v%d.%d.%d-nightly.%s", base.Major, base.Minor, base.Patch+1, date),
	}

	newest, ok := highestTag(tags, isNightly)
	if !ok {
		return plan, nil
	}
	plan.Newest = newest
	if today, ok := highestTag(tags, func(v Version) bool { return isNightly(v) && v.Pre[1] == date }); ok {
		plan.Tag = today
	}
	switch Compare(plan.Tag, newest) {
	case -1:
		return plan, fmt.Errorf("next nightly %s is lower than the existing nightly %s", plan.Tag, newest)
	case 0:
		plan.Exists = true
	}
	return plan, nil
}

// NightlyDeletions returns the nightly tags beyond the newest keep, oldest
// first. Tags that are not nightlies are never returned.
func NightlyDeletions(tags []string, keep int) ([]string, error) {
	if keep < 1 {
		return nil, fmt.Errorf("keep must be 1 or more, got %d", keep)
	}
	var nightlies []string
	for _, tag := range tags {
		if v, ok := ParseVersion(tag); ok && isNightly(v) {
			nightlies = append(nightlies, tag)
		}
	}
	slices.SortFunc(nightlies, Compare)
	nightlies = slices.Compact(nightlies)
	excess := len(nightlies) - keep
	if excess <= 0 {
		return nil, nil
	}
	return nightlies[:excess], nil
}

// DistTagChanges returns the npm dist-tag moves that make each existing
// nightly, alpha, beta and rc dist-tag point at the highest published version
// in that track's upgrade path. A dist-tag that does not exist is not created,
// and "latest" is never touched. Versions may be given with or without "v"; the
// emitted versions never have one.
func DistTagChanges(published []string, distTags map[string]string) []DistTagChange {
	var changes []DistTagChange
	for _, track := range distTagTracks {
		current, exists := distTags[track.DistTag()]
		if !exists {
			continue
		}
		target, ok := HighestInPath(track, published)
		if !ok || Compare(current, target) == 0 {
			continue
		}
		changes = append(changes, DistTagChange{Tag: track.DistTag(), Version: strings.TrimPrefix(target, "v")})
	}
	return changes
}

func classify(v Version) (Track, bool) {
	if len(v.Pre) == 0 {
		return Stable, true
	}
	switch t := Track(v.Pre[0]); t {
	case Nightly, Alpha, Beta, RC:
		return t, true
	}
	return "", false
}

func inTracks(tracks []Track) func(Version) bool {
	return func(v Version) bool {
		t, ok := classify(v)
		if t == Nightly && !isNightly(v) {
			return false
		}
		return ok && slices.Contains(tracks, t)
	}
}

func isNightly(v Version) bool {
	return len(v.Pre) == 2 && v.Pre[0] == string(Nightly) &&
		len(v.Pre[1]) == len(NightlyDateLayout) && isNumeric(v.Pre[1])
}

func highestTag(tags []string, accept func(Version) bool) (string, bool) {
	var best string
	var bestVersion Version
	found := false
	for _, tag := range tags {
		v, ok := ParseVersion(tag)
		if !ok || !accept(v) {
			continue
		}
		if !found || v.Compare(bestVersion) > 0 {
			best, bestVersion, found = tag, v, true
		}
	}
	return best, found
}
