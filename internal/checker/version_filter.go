package checker

import (
	"regexp"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// FilterMode controls how versions are filtered by channel.
type FilterMode int

const (
	// FilterAuto filters based on the current version's channel:
	// stable shows only stable, prerelease shows same channel + stable.
	FilterAuto FilterMode = iota
	// FilterStable shows only stable versions.
	FilterStable
	// FilterAll shows all versions regardless of channel.
	FilterAll
)

var (
	archTokens = map[string]bool{
		"amd64":   true,
		"arm64":   true,
		"arm64v8": true,
		"arm32v7": true,
		"386":     true,
		"s390x":   true,
		"ppc64le": true,
		"riscv64": true,
	}

	channelTokens = map[string]bool{
		"alpha":    true,
		"beta":     true,
		"rc":       true,
		"nightly":  true,
		"dev":      true,
		"develop":  true,
		"snapshot": true,
		"testing":  true,
		"unstable": true,
	}

	// lsBuildRegex matches linuxserver.io build iteration tags like "ls141".
	lsBuildRegex = regexp.MustCompile(`^ls\d+$`)

	// pureNumericRegex matches purely numeric tokens like "5327", "9".
	pureNumericRegex = regexp.MustCompile(`^\d+$`)

	// tagPrefixRegex matches architecture and other non-version prefixes.
	tagPrefixRegex = regexp.MustCompile(`^(amd64|arm64v8|arm64|arm32v7|386|s390x|ppc64le|riscv64|version)-`)

	// compoundVersionRegex matches tags where a short version prefix (X.Y) is
	// followed by a full version after a dash, e.g., "5.14-2.0.0.5344-ls5".
	// This is linuxserver's legacy format: <runtime-version>-<app-version>-<ls-build>.
	compoundVersionRegex = regexp.MustCompile(`^(\d+\.\d+)-(\d+\.\d+\.\d+.*)$`)
)

// NormalizeVersion pre-processes a version tag before semver parsing:
//   - Strips architecture prefixes (e.g., "amd64-6.1.1.10360-ls299" → "6.1.1.10360-ls299")
//   - Extracts app version from compound tags (e.g., "5.14-2.0.0.5344-ls5" → "2.0.0-5344.ls5")
//   - Coerces 4+ segment versions by moving extra segments into the prerelease
//     (e.g., "2.3.5.5327-ls141" → "2.3.5-5327.ls141")
func NormalizeVersion(tag string) string {
	// Strip tag prefixes (arch, "version-", etc.)
	tag = tagPrefixRegex.ReplaceAllString(tag, "")

	// Extract app version from compound tags (runtime-version + app-version)
	if m := compoundVersionRegex.FindStringSubmatch(tag); m != nil {
		tag = m[2] // use the app version part
	}

	// Coerce 4+ segment versions
	tag = coerceExtraSegments(tag)

	return tag
}

// coerceExtraSegments converts versions with more than 3 numeric segments
// by moving segments 4+ into the prerelease portion.
// "2.3.5.5327-ls141" → "2.3.5-5327.ls141"
// "2.3.5.5327" → "2.3.5-5327"
func coerceExtraSegments(tag string) string {
	// Strip optional "v" prefix for parsing, add back later
	prefix := ""
	work := tag
	if strings.HasPrefix(work, "v") {
		prefix = "v"
		work = work[1:]
	}

	// Split off existing prerelease/metadata
	var prerelease string
	if idx := strings.IndexByte(work, '-'); idx >= 0 {
		prerelease = work[idx+1:]
		work = work[:idx]
	}

	// Split version segments
	segments := strings.Split(work, ".")
	if len(segments) <= 3 {
		return tag // no coercion needed
	}

	// Take first 3 as major.minor.patch, rest becomes prerelease
	base := strings.Join(segments[:3], ".")
	extra := strings.Join(segments[3:], ".")

	var result string
	if prerelease != "" {
		result = prefix + base + "-" + extra + "." + prerelease
	} else {
		result = prefix + base + "-" + extra
	}

	return result
}

// DetectChannel determines the release channel of a semver version.
// Returns "" for stable, or the channel name (e.g., "beta", "rc", "nightly").
func DetectChannel(v *semver.Version) string {
	pre := v.Prerelease()
	if pre == "" {
		return ""
	}

	// Split prerelease into tokens on "-" and "."
	tokens := splitPrerelease(pre)

	// Find the first token that matches a known channel
	for _, token := range tokens {
		lower := strings.ToLower(token)
		if channelTokens[lower] {
			return lower
		}
	}

	return ""
}

// splitPrerelease splits a prerelease string into tokens, filtering out
// noise: architecture tags, pure numeric segments, and linuxserver build tags.
func splitPrerelease(pre string) []string {
	// Split on both "-" and "."
	raw := strings.FieldsFunc(pre, func(r rune) bool {
		return r == '-' || r == '.'
	})

	var tokens []string
	for _, t := range raw {
		lower := strings.ToLower(t)
		// Skip known noise
		if archTokens[lower] {
			continue
		}
		if pureNumericRegex.MatchString(t) {
			continue
		}
		if lsBuildRegex.MatchString(lower) {
			continue
		}
		tokens = append(tokens, t)
	}

	return tokens
}

// FilterVersions filters a sorted version list based on the current version's
// channel and the specified filter mode.
func FilterVersions(versions []*semver.Version, current *semver.Version, mode FilterMode) []*semver.Version {
	if mode == FilterAll {
		return versions
	}

	var currentChannel string
	if current != nil {
		currentChannel = DetectChannel(current)
	}

	if mode == FilterStable {
		currentChannel = ""
	}

	var filtered []*semver.Version
	for _, v := range versions {
		ch := DetectChannel(v)
		if currentChannel == "" {
			// Stable: only keep stable versions
			if ch == "" {
				filtered = append(filtered, v)
			}
		} else {
			// Prerelease: keep same channel + stable
			if ch == currentChannel || ch == "" {
				filtered = append(filtered, v)
			}
		}
	}

	return filtered
}

// TaggedVersion pairs a parsed semver version with its original raw tag.
type TaggedVersion struct {
	Version *semver.Version
	Tag     string // original raw tag from the registry
}

// ParseVersion normalizes a version tag and parses it as semver.
// Returns nil if the tag cannot be parsed.
func ParseVersion(tag string) *semver.Version {
	normalized := NormalizeVersion(tag)
	if normalized == "" {
		return nil
	}
	v, err := semver.NewVersion(normalized)
	if err != nil {
		return nil
	}
	return v
}

// ParseTaggedVersions normalizes and parses a list of tags, deduplicating by
// the normalized version string. Returns sorted newest first.
// Each result preserves the original raw tag for display, preferring the
// shortest (cleanest) tag when duplicates exist (e.g., "2.0.0.5344-ls84"
// over "amd64-2.0.0.5344-ls84").
func ParseTaggedVersions(tags []string) []TaggedVersion {
	seen := make(map[string]int) // key → index in versions slice
	var versions []TaggedVersion
	for _, tag := range tags {
		v := ParseVersion(tag)
		if v == nil {
			continue
		}
		key := v.String()
		if idx, exists := seen[key]; exists {
			// Prefer the shorter (cleaner) tag
			if len(tag) < len(versions[idx].Tag) {
				versions[idx].Tag = tag
			}
			continue
		}
		seen[key] = len(versions)
		versions = append(versions, TaggedVersion{Version: v, Tag: tag})
	}
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Version.GreaterThan(versions[j].Version)
	})
	return versions
}

// FilterTaggedVersions filters a sorted tagged version list based on the
// current version's channel and the specified filter mode.
func FilterTaggedVersions(versions []TaggedVersion, current *semver.Version, mode FilterMode) []TaggedVersion {
	if mode == FilterAll {
		return versions
	}

	var currentChannel string
	if current != nil {
		currentChannel = DetectChannel(current)
	}

	if mode == FilterStable {
		currentChannel = ""
	}

	var filtered []TaggedVersion
	for _, tv := range versions {
		ch := DetectChannel(tv.Version)
		if currentChannel == "" {
			if ch == "" {
				filtered = append(filtered, tv)
			}
		} else {
			if ch == currentChannel || ch == "" {
				filtered = append(filtered, tv)
			}
		}
	}

	return filtered
}
