package release_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/m10rten/go-canary-cli/internal/release"
)

func TestTrackOf(t *testing.T) {
	testcases := []struct {
		version string
		want    release.Track
	}{
		{"v1.2.3", release.Stable},
		{"1.2.3", release.Stable},
		{"v1.2.3-nightly.20261025", release.Nightly},
		{"v1.2.3-alpha.1", release.Alpha},
		{"v1.2.3-beta.10", release.Beta},
		{"v1.2.3-rc.1", release.RC},
		{"v1.2.3-rc.1+build.5", release.RC},
		{"v0.0.0-ci", release.Stable},
		{"v0.1.0-3-g4f2a9c1", release.Stable},
		{"v0.1.0-3-g4f2a9c1-dirty", release.Stable},
		{"4f2a9c1", release.Stable},
		{"dev", release.Stable},
		{"", release.Stable},
		{"v1.2.3-RC.1", release.Stable},
		{"v1.2.3-rc-1", release.Stable},
	}

	for _, tc := range testcases {
		t.Run(tc.version, func(t *testing.T) {
			if got := release.TrackOf(tc.version); got != tc.want {
				t.Errorf("TrackOf(%q) = %q, want %q", tc.version, got, tc.want)
			}
		})
	}
}

func TestTrackPath(t *testing.T) {
	testcases := []struct {
		track      release.Track
		want       []release.Track
		distTag    string
		prerelease bool
	}{
		{release.Stable, []release.Track{release.Stable}, "latest", false},
		{release.Nightly, []release.Track{release.Nightly, release.Stable}, "nightly", true},
		{release.RC, []release.Track{release.RC, release.Stable}, "rc", true},
		{release.Beta, []release.Track{release.Beta, release.RC, release.Stable}, "beta", true},
		{release.Alpha, []release.Track{release.Alpha, release.Beta, release.RC, release.Stable}, "alpha", true},
	}

	for _, tc := range testcases {
		t.Run(string(tc.track), func(t *testing.T) {
			if got := tc.track.Path(); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Path() = %v, want %v", got, tc.want)
			}
			if got := tc.track.DistTag(); got != tc.distTag {
				t.Errorf("DistTag() = %q, want %q", got, tc.distTag)
			}
			if got := tc.track.Prerelease(); got != tc.prerelease {
				t.Errorf("Prerelease() = %v, want %v", got, tc.prerelease)
			}
		})
	}
}

func TestSelectTarget(t *testing.T) {
	testcases := []struct {
		name     string
		current  string
		releases []string
		want     string
	}{
		{"nightly current, newer stable exists: offer the stable", "v1.2.4-nightly.20261020", []string{"v1.2.4-nightly.20261022", "v1.2.4", "v1.2.3"}, "v1.2.4"},
		{"nightly current, newest nightly above newest stable: offer the nightly", "v1.2.4-nightly.20261020", []string{"v1.2.3", "v1.2.4-nightly.20261022", "v1.2.4-nightly.20261021"}, "v1.2.4-nightly.20261022"},
		{"nightly current ignores other prereleases", "v1.2.4-nightly.20261020", []string{"v1.3.0-rc.1", "v1.3.0-beta.1", "v1.3.0-alpha.1"}, ""},
		{"beta to rc", "v1.3.0-beta.1", []string{"v1.3.0-rc.1", "v1.3.0-beta.1"}, "v1.3.0-rc.1"},
		{"beta to a newer beta", "v1.3.0-beta.1", []string{"v1.3.0-beta.2", "v1.3.0-beta.1"}, "v1.3.0-beta.2"},
		{"rc to stable", "v1.3.0-rc.1", []string{"v1.3.0", "v1.3.0-rc.2"}, "v1.3.0"},
		{"rc to a newer rc", "v1.3.0-rc.1", []string{"v1.3.0-rc.2", "v1.2.0"}, "v1.3.0-rc.2"},
		{"beta never to alpha", "v1.3.0-beta.1", []string{"v1.3.0-alpha.9", "v1.4.0-alpha.1"}, ""},
		{"alpha to beta", "v1.3.0-alpha.1", []string{"v1.3.0-beta.1"}, "v1.3.0-beta.1"},
		{"alpha to rc over beta", "v1.3.0-alpha.1", []string{"v1.3.0-beta.1", "v1.3.0-rc.1"}, "v1.3.0-rc.1"},
		{"alpha to stable over rc", "v1.3.0-alpha.1", []string{"v1.3.0-rc.1", "v1.3.0", "v1.3.0-nightly.20261025"}, "v1.3.0"},
		{"stable never to a prerelease", "v1.2.3", []string{"v1.3.0-rc.1", "v1.3.0-beta.1", "v1.3.0-alpha.1", "v1.3.0-nightly.20261025"}, ""},
		{"stable to a newer stable", "v1.2.3", []string{"v1.3.0-rc.1", "v1.2.4", "v1.2.2"}, "v1.2.4"},
		{"nightly current is never offered an odd nightly shape", "v1.0.0-nightly.20261020", []string{"v9.0.0-nightly.1", "v9.0.0-nightly.20261025.x"}, ""},
		{"no non-nightly track is offered a nightly", "v1.0.0-alpha.1", []string{"v9.0.0-nightly.20261025"}, ""},
		{"up to date on the newest stable", "v1.2.4", []string{"v1.2.4", "v1.2.3"}, ""},
		{"up to date on the newest nightly", "v1.2.4-nightly.20261022", []string{"v1.2.4-nightly.20261022", "v1.2.3"}, ""},
		{"nightly above every stable in the list", "v1.2.4-nightly.20261022", []string{"v1.2.4-nightly.20261021", "v1.2.3"}, ""},
		{"build metadata on current is ignored", "v1.2.4+abc", []string{"v1.2.4"}, ""},
		{"empty list", "v1.2.3", nil, ""},
		{"unparseable and unknown tags are ignored", "v1.2.3", []string{"latest", "vfoo", "v9.0.0-ci", "v9.0.0-foo.1", "v1.2.4"}, "v1.2.4"},
		{"dev build is offered the newest stable", "dev", []string{"v1.2.3", "v1.3.0-rc.1"}, "v1.2.3"},
		{"v0.0.0-ci is stable", "v0.0.0-ci", []string{"v0.1.0", "v0.1.1-rc.1"}, "v0.1.0"},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := release.SelectTarget(tc.current, tc.releases)
			if got != tc.want || ok != (tc.want != "") {
				t.Errorf("SelectTarget(%q, %v) = %q, %v, want %q", tc.current, tc.releases, got, ok, tc.want)
			}
		})
	}
}

func TestHighestInPath(t *testing.T) {
	testcases := []struct {
		name     string
		track    release.Track
		releases []string
		want     string
	}{
		{"nightly path, newest stable above newest nightly: the stable", release.Nightly, []string{"v1.2.4-nightly.20261022", "v1.2.4", "v1.2.4-nightly.20261021"}, "v1.2.4"},
		{"nightly path, newest nightly above newest stable: the nightly", release.Nightly, []string{"v1.2.3", "v1.2.4-nightly.20261022", "v1.2.4-nightly.20261021"}, "v1.2.4-nightly.20261022"},
		{"beta path skips alpha and nightly", release.Beta, []string{"v1.3.0-alpha.1", "v1.3.0-nightly.20261025", "v1.3.0-beta.1"}, "v1.3.0-beta.1"},
		{"stable path", release.Stable, []string{"v1.3.0-rc.1", "v1.2.3"}, "v1.2.3"},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := release.HighestInPath(tc.track, tc.releases)
			if got != tc.want || ok != (tc.want != "") {
				t.Errorf("HighestInPath(%q, %v) = %q, %v, want %q", tc.track, tc.releases, got, ok, tc.want)
			}
		})
	}
}

func TestValidatePushedTag(t *testing.T) {
	testcases := []struct {
		tag     string
		want    release.Track
		wantErr string
	}{
		{tag: "v1.2.3", want: release.Stable},
		{tag: "v0.2.0", want: release.Stable},
		{tag: "v1.3.0-rc.1", want: release.RC},
		{tag: "v1.3.0-beta.10", want: release.Beta},
		{tag: "v1.3.0-alpha.2", want: release.Alpha},
		{tag: "v1.3.0-rc.0", wantErr: "N of 1 or more"},
		{tag: "v1.3.0-rc.01", wantErr: "not a valid semantic version"},
		{tag: "v1.3.0-rc", wantErr: "N of 1 or more"},
		{tag: "v1.3.0-rc.1.2", wantErr: "N of 1 or more"},
		{tag: "v1.3.0-rc.x", wantErr: "N of 1 or more"},
		{tag: "v1.3.0-nightly.20261025", wantErr: "nightly"},
		{tag: "v1.3.0-ci", wantErr: "unknown prerelease"},
		{tag: "v1.3.0+build", wantErr: "build metadata"},
		{tag: "v1.3.0-rc.1+build", wantErr: "build metadata"},
		{tag: "1.3.0", wantErr: "must start with v"},
		{tag: "vfoo", wantErr: "not a valid semantic version"},
		{tag: "v1.3", wantErr: "not a valid semantic version"},
		{tag: "", wantErr: "must start with v"},
	}

	for _, tc := range testcases {
		t.Run(tc.tag, func(t *testing.T) {
			got, err := release.ValidatePushedTag(tc.tag)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("ValidatePushedTag(%q) error = %v, want containing %q", tc.tag, err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("ValidatePushedTag(%q) = %q, %v, want %q", tc.tag, got, err, tc.want)
			}
		})
	}
}

func TestNextNightly(t *testing.T) {
	day := time.Date(2026, 10, 25, 23, 59, 0, 0, time.UTC)

	testcases := []struct {
		name    string
		tags    []string
		now     time.Time
		want    release.NightlyPlan
		wantErr string
	}{
		{"no releases", nil, day, release.NightlyPlan{Tag: "v0.0.1-nightly.20261025"}, ""},
		{"patch bump from the highest stable", []string{"v1.2.3", "v1.2.2"}, day, release.NightlyPlan{Tag: "v1.2.4-nightly.20261025"}, ""},
		{"backport does not lower the base", []string{"v1.3.0", "v1.2.9"}, day, release.NightlyPlan{Tag: "v1.3.1-nightly.20261025"}, ""},
		{"prereleases are not a base", []string{"v1.2.3", "v1.3.0-rc.1", "v1.3.0-beta.1"}, day, release.NightlyPlan{Tag: "v1.2.4-nightly.20261025"}, ""},
		{"unparseable tags are ignored", []string{"v1.2.3", "latest", "vfoo"}, day, release.NightlyPlan{Tag: "v1.2.4-nightly.20261025"}, ""},
		{"date is taken in UTC", []string{"v1.2.3"}, time.Date(2026, 10, 25, 23, 30, 0, 0, time.FixedZone("UTC-2", -2*3600)), release.NightlyPlan{Tag: "v1.2.4-nightly.20261026"}, ""},
		{"newer than the existing nightly", []string{"v1.2.3", "v1.2.4-nightly.20261024"}, day, release.NightlyPlan{Tag: "v1.2.4-nightly.20261025", Newest: "v1.2.4-nightly.20261024"}, ""},
		{"stable released since the last nightly raises the base", []string{"v1.2.4", "v1.2.4-nightly.20261024"}, day, release.NightlyPlan{Tag: "v1.2.5-nightly.20261025", Newest: "v1.2.4-nightly.20261024"}, ""},
		{"stable released the same day returns today's nightly", []string{"v1.2.4", "v1.2.4-nightly.20261025"}, day, release.NightlyPlan{Tag: "v1.2.4-nightly.20261025", Newest: "v1.2.4-nightly.20261025", Exists: true}, ""},
		{"today's tag already exists", []string{"v1.2.3", "v1.2.4-nightly.20261024", "v1.2.4-nightly.20261025"}, day, release.NightlyPlan{Tag: "v1.2.4-nightly.20261025", Newest: "v1.2.4-nightly.20261025", Exists: true}, ""},
		{"lower than the newest nightly", []string{"v1.2.3", "v1.2.4-nightly.20261026"}, day, release.NightlyPlan{Tag: "v1.2.4-nightly.20261025", Newest: "v1.2.4-nightly.20261026"}, "lower than the existing nightly v1.2.4-nightly.20261026"},
		{"lower than a nightly of a higher base", []string{"v1.2.3", "v1.2.5-nightly.20261001"}, day, release.NightlyPlan{Tag: "v1.2.4-nightly.20261025", Newest: "v1.2.5-nightly.20261001"}, "lower than the existing nightly"},
		{"odd nightly shapes are not nightlies", []string{"v1.2.3", "v1.2.4-nightly.1", "v1.2.4-nightly.20261025.x"}, day, release.NightlyPlan{Tag: "v1.2.4-nightly.20261025"}, ""},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := release.NextNightly(tc.tags, tc.now)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("NextNightly = %+v, %v, want %+v", got, err, tc.want)
			}
		})
	}
}

func TestNightlyDeletions(t *testing.T) {
	nightlies := func(days ...string) []string {
		var tags []string
		for _, d := range days {
			tags = append(tags, "v1.2.4-nightly.202610"+d)
		}
		return tags
	}
	mixed := append([]string{"v1.2.3", "v1.3.0-rc.1", "v1.3.0-beta.1", "v1.2.4-nightly.1", "latest"}, nightlies("05", "03", "04", "01", "02")...)

	testcases := []struct {
		name    string
		tags    []string
		keep    int
		want    []string
		wantErr bool
	}{
		{"fewer than keep", nightlies("01", "02"), 14, nil, false},
		{"exactly keep", nightlies("01", "02", "03"), 3, nil, false},
		{"one beyond keep", nightlies("03", "01", "02", "04"), 3, nightlies("01"), false},
		{"more than keep with others mixed in, oldest first", mixed, 2, nightlies("01", "02", "03"), false},
		{"nothing but non-nightlies", []string{"v1.2.3", "v1.3.0-rc.1"}, 1, nil, false},
		{"keep zero is rejected", nightlies("01"), 0, nil, true},
		{"negative keep is rejected", nightlies("01"), -1, nil, true},
		{"duplicated newest tag is not returned", append(nightlies("03", "02", "01"), nightlies("03")...), 1, nightlies("01", "02"), false},
		{"nightlies across a base bump sort by semver", []string{"v1.2.5-nightly.20261001", "v1.2.4-nightly.20261020"}, 1, []string{"v1.2.4-nightly.20261020"}, false},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := release.NightlyDeletions(tc.tags, tc.keep)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("NightlyDeletions = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDistTagChanges(t *testing.T) {
	allTags := func(v string) map[string]string {
		return map[string]string{"latest": "1.2.3", "nightly": v, "alpha": v, "beta": v, "rc": v}
	}

	testcases := []struct {
		name      string
		published []string
		distTags  map[string]string
		want      []release.DistTagChange
	}{
		{
			name:      "stable above the newest nightly moves nightly, alpha, beta and rc",
			published: []string{"1.2.3", "1.2.4-nightly.20261024", "1.2.4"},
			distTags:  allTags("1.2.4-nightly.20261024"),
			want: []release.DistTagChange{
				{Tag: "nightly", Version: "1.2.4"}, {Tag: "alpha", Version: "1.2.4"},
				{Tag: "beta", Version: "1.2.4"}, {Tag: "rc", Version: "1.2.4"},
			},
		},
		{
			name:      "a newer nightly takes nightly back",
			published: []string{"1.2.4", "1.2.5-nightly.20261025"},
			distTags:  map[string]string{"latest": "1.2.4", "nightly": "1.2.4"},
			want:      []release.DistTagChange{{Tag: "nightly", Version: "1.2.5-nightly.20261025"}},
		},
		{
			name:      "a lower nightly published late does not win",
			published: []string{"1.2.3", "1.2.4-nightly.20261025", "1.2.4-nightly.20261020"},
			distTags:  map[string]string{"latest": "1.2.3", "nightly": "1.2.4-nightly.20261025"},
			want:      nil,
		},
		{
			name:      "a late publish that pulled nightly down is repaired",
			published: []string{"1.2.3", "1.2.4-nightly.20261025", "1.2.4-nightly.20261020"},
			distTags:  map[string]string{"latest": "1.2.3", "nightly": "1.2.4-nightly.20261020"},
			want:      []release.DistTagChange{{Tag: "nightly", Version: "1.2.4-nightly.20261025"}},
		},
		{
			name:      "an rc moves alpha and beta but not nightly",
			published: []string{"1.2.3", "1.2.4-nightly.20261024", "1.3.0-rc.1"},
			distTags: map[string]string{
				"latest": "1.2.3", "nightly": "1.2.4-nightly.20261024",
				"alpha": "1.3.0-alpha.1", "beta": "1.3.0-beta.1", "rc": "1.3.0-rc.1",
			},
			want: []release.DistTagChange{{Tag: "alpha", Version: "1.3.0-rc.1"}, {Tag: "beta", Version: "1.3.0-rc.1"}},
		},
		{
			name:      "a dist-tag that does not exist is not created",
			published: []string{"1.2.3", "1.2.4-nightly.20261024", "1.3.0-beta.1"},
			distTags:  map[string]string{"latest": "1.2.3", "nightly": "1.2.4-nightly.20261024"},
			want:      nil,
		},
		{
			name:      "latest is never changed",
			published: []string{"1.2.3", "1.2.4", "1.3.0-rc.1"},
			distTags:  map[string]string{"latest": "1.2.3"},
			want:      nil,
		},
		{
			name:      "a re-run after the reconcile changes nothing",
			published: []string{"1.2.3", "1.2.4-nightly.20261024", "1.2.4"},
			distTags:  allTags("1.2.4"),
			want:      nil,
		},
		{
			name:      "alpha does not take a nightly",
			published: []string{"1.2.3", "1.3.0-alpha.1", "1.3.0-nightly.20261025"},
			distTags:  map[string]string{"latest": "1.2.3", "alpha": "1.3.0-alpha.1"},
			want:      nil,
		},
		{
			name:      "versions given with a v prefix compare equal",
			published: []string{"v1.2.4"},
			distTags:  map[string]string{"nightly": "1.2.4"},
			want:      nil,
		},
		{
			name:      "emitted versions never have a v prefix",
			published: []string{"v1.2.4"},
			distTags:  map[string]string{"nightly": "1.2.3-nightly.20261024"},
			want:      []release.DistTagChange{{Tag: "nightly", Version: "1.2.4"}},
		},
		{
			name:      "nothing published in the path leaves the dist-tag alone",
			published: []string{"1.3.0-rc.1"},
			distTags:  map[string]string{"nightly": "1.2.4-nightly.20261024"},
			want:      nil,
		},
		{
			name:      "no published versions",
			published: nil,
			distTags:  allTags("1.2.4"),
			want:      nil,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if got := release.DistTagChanges(tc.published, tc.distTags); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("DistTagChanges = %v, want %v", got, tc.want)
			}
		})
	}
}
