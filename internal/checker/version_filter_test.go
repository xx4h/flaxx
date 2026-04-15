package checker

import (
	"testing"

	"github.com/Masterminds/semver/v3"
)

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"v1.20.0", "v1.20.0"},
		{"v1.20.0-beta.0", "v1.20.0-beta.0"},
		{"v1.20.0-alpha.1", "v1.20.0-alpha.1"},
		{"v1.76.0", "v1.76.0"},
		{"v1.76", "v1.76"},
		{"2.3.5.5327-ls141", "2.3.5-5327.ls141"},
		{"2.3.6-nightly", "2.3.6-nightly"},
		{"7.0.0-9-arm64", "7.0.0-9-arm64"},
		{"7.0.0-9", "7.0.0-9"},
		{"2026041305", "2026041305"},
		{"2026040605-amd64", "2026040605-amd64"},
		{"amd64-6.1.1.10360-ls299", "6.1.1-10360.ls299"},
		// 4-segment without prerelease
		{"2.3.5.5327", "2.3.5-5327"},
		// arch prefix with normal version
		{"arm64v8-1.2.3", "1.2.3"},
		// v-prefix with 4 segments
		{"v2.3.5.5327-ls141", "v2.3.5-5327.ls141"},
		// version- prefix
		{"version-3.0.4.993", "3.0.4-993"},
		// arm32v7 prefix
		{"arm32v7-2.0.0.5344-ls84", "2.0.0-5344.ls84"},
		// Compound version: runtime prefix stripped, app version extracted
		{"5.14-2.0.0.5344-ls5", "2.0.0-5344.ls5"},
		{"5.14-2.0.0.5344-ls15", "2.0.0-5344.ls15"},
		// Compound with arch prefix
		{"amd64-5.14-2.0.0.5344-ls5", "2.0.0-5344.ls5"},
		// Compound: 5.14 alone is not compound (no app version after dash)
		{"5.14", "5.14"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeVersion(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeVersion(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDetectChannel(t *testing.T) {
	tests := []struct {
		version string
		want    string
	}{
		{"1.2.3", ""},
		{"1.2.3-beta.1", "beta"},
		{"1.2.3-rc.1", "rc"},
		{"1.2.3-alpha", "alpha"},
		{"1.2.3-nightly", "nightly"},
		{"1.2.3-dev", "dev"},
		{"1.2.3-develop", "develop"},
		{"1.2.3-testing", "testing"},
		{"1.2.3-unstable", "unstable"},
		{"1.2.3-snapshot", "snapshot"},
		// Arch and numeric noise stripped
		{"1.2.3-9-arm64", ""},
		{"1.2.3-amd64", ""},
		{"1.2.3-9", ""},
		// Linuxserver build iteration
		{"2.3.5-5327.ls141", ""},
		// Channel mixed with noise
		{"1.2.3-9-nightly-arm64", "nightly"},
		{"1.2.3-arm64-nightly", "nightly"},
		// Pure numeric prerelease
		{"7.0.0-9", ""},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			v, err := semver.NewVersion(tt.version)
			if err != nil {
				t.Fatalf("failed to parse %q: %v", tt.version, err)
			}
			got := DetectChannel(v)
			if got != tt.want {
				t.Errorf("DetectChannel(%q) = %q, want %q", tt.version, got, tt.want)
			}
		})
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		tag      string
		wantNil  bool
		wantOrig string
	}{
		{"v1.20.0", false, "v1.20.0"},
		{"2.3.5.5327-ls141", false, "2.3.5-5327.ls141"},
		{"amd64-6.1.1.10360-ls299", false, "6.1.1-10360.ls299"},
		{"not-a-version", true, ""},
		{"latest", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			v := ParseVersion(tt.tag)
			if tt.wantNil && v != nil {
				t.Errorf("ParseVersion(%q) should be nil, got %v", tt.tag, v)
			}
			if !tt.wantNil && v == nil {
				t.Errorf("ParseVersion(%q) should not be nil", tt.tag)
			}
		})
	}
}

func TestFilterVersions_AutoStable(t *testing.T) {
	current := mustParse("1.0.0")
	versions := []*semver.Version{
		mustParse("1.2.0-rc.1"),
		mustParse("1.1.0-beta.1"),
		mustParse("1.1.0"),
		mustParse("1.0.1"),
	}

	filtered := FilterVersions(versions, current, FilterAuto)

	// Should only keep stable versions
	if len(filtered) != 2 {
		t.Fatalf("got %d versions, want 2: %v", len(filtered), originals(filtered))
	}
	for _, v := range filtered {
		if DetectChannel(v) != "" {
			t.Errorf("stable filter should not include %q (channel: %q)", v.Original(), DetectChannel(v))
		}
	}
}

func TestFilterVersions_AutoPrerelease(t *testing.T) {
	current := mustParse("1.0.0-beta.1")
	versions := []*semver.Version{
		mustParse("1.1.0"),
		mustParse("1.0.0-rc.1"),
		mustParse("1.0.0-beta.2"),
		mustParse("1.0.0-alpha.1"),
	}

	filtered := FilterVersions(versions, current, FilterAuto)

	// Should keep beta + stable, not rc or alpha
	want := map[string]bool{
		"1.1.0":        true,
		"1.0.0-beta.2": true,
	}
	if len(filtered) != len(want) {
		t.Fatalf("got %d versions, want %d: %v", len(filtered), len(want), originals(filtered))
	}
	for _, v := range filtered {
		if !want[v.Original()] {
			t.Errorf("unexpected version %q in filtered results", v.Original())
		}
	}
}

func TestFilterVersions_Stable(t *testing.T) {
	current := mustParse("1.0.0-beta.1")
	versions := []*semver.Version{
		mustParse("1.1.0"),
		mustParse("1.0.0-beta.2"),
		mustParse("1.0.0-rc.1"),
	}

	filtered := FilterVersions(versions, current, FilterStable)

	// Should only keep stable, even though current is beta
	if len(filtered) != 1 {
		t.Fatalf("got %d versions, want 1: %v", len(filtered), originals(filtered))
	}
	if filtered[0].Original() != "1.1.0" {
		t.Errorf("got %q, want %q", filtered[0].Original(), "1.1.0")
	}
}

func TestFilterVersions_All(t *testing.T) {
	current := mustParse("1.0.0")
	versions := []*semver.Version{
		mustParse("1.1.0"),
		mustParse("1.0.0-beta.2"),
		mustParse("1.0.0-rc.1"),
	}

	filtered := FilterVersions(versions, current, FilterAll)

	// Should keep everything
	if len(filtered) != 3 {
		t.Fatalf("got %d versions, want 3: %v", len(filtered), originals(filtered))
	}
}

func mustParse(v string) *semver.Version {
	ver, err := semver.NewVersion(v)
	if err != nil {
		panic("invalid test version: " + v)
	}
	return ver
}

func originals(versions []*semver.Version) []string {
	var result []string
	for _, v := range versions {
		result = append(result, v.Original())
	}
	return result
}
