package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var gaTagRe = regexp.MustCompile(`^(\d+)\.(\d+)\.0$`)

type version struct {
	major, minor int
	original     string
}

func parseGA(tag string) (version, error) {
	tag = strings.TrimPrefix(strings.TrimSpace(tag), "v")
	m := gaTagRe.FindStringSubmatch(tag)
	if m == nil {
		return version{}, fmt.Errorf("tag %q is not an x.y.0 GA tag", tag)
	}
	maj, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	return version{major: maj, minor: min, original: m[0]}, nil
}

func compareVer(a, b version) int {
	if a.major != b.major {
		return a.major - b.major
	}
	return a.minor - b.minor
}

// eolRelease returns "release-x.y" for the GA tag two slots before newTag.
// ok is false when there is not enough history (caller should no-op and exit 0).
func eolRelease(newTag string, tags []string) (eolBranch string, ok bool, err error) {
	target, err := parseGA(newTag)
	if err != nil {
		return "", false, err
	}

	seen := map[string]version{target.original: target}
	for _, t := range tags {
		v, perr := parseGA(t)
		if perr != nil {
			continue
		}
		seen[v.original] = v
	}

	vs := make([]version, 0, len(seen))
	for _, v := range seen {
		vs = append(vs, v)
	}
	sort.Slice(vs, func(i, j int) bool { return compareVer(vs[i], vs[j]) < 0 })

	idx := -1
	for i, v := range vs {
		if v.original == target.original {
			idx = i
			break
		}
	}
	if idx < 2 {
		return "", false, nil
	}
	eol := vs[idx-2]
	return fmt.Sprintf("release-%d.%d", eol.major, eol.minor), true, nil
}
