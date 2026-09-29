// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"regexp"
	"strconv"
	"strings"
)

// getRegex compiles a package name or pattern into an anchored regular
// expression. Go's regexp.MatchString searches anywhere in the string, so
// every branch is explicitly anchored at the start with "^".
func getRegex(pkg string, insensitive, isRegex bool) (*regexp.Regexp, error) {
	var body string
	fullMatch := false

	switch {
	case isRegex:
		body = pkg
	case strings.ContainsAny(pkg, "[?*"):
		body = globToRegexBody(pkg)
		fullMatch = true
	default:
		// Not escaped: any regex metacharacters in a plain package name are
		// passed through as-is.
		body = pkg + "$"
	}

	pattern := "^(?:" + body + ")"
	if fullMatch {
		pattern += "$"
	}
	if insensitive {
		pattern = "(?i)" + pattern
	}
	return regexp.Compile(pattern)
}

// globToRegexBody translates a shell glob pattern into the body of a Go
// regular expression. It supports "*", "?" and "[seq]"/"[!seq]" character
// classes, with everything else treated literally.
func globToRegexBody(pattern string) string {
	var sb strings.Builder
	i, n := 0, len(pattern)
	for i < n {
		c := pattern[i]
		i++
		switch c {
		case '*':
			sb.WriteString(".*")
		case '?':
			sb.WriteString(".")
		case '[':
			j := i
			if j < n && (pattern[j] == '!' || pattern[j] == '^') {
				j++
			}
			if j < n && pattern[j] == ']' {
				j++
			}
			for j < n && pattern[j] != ']' {
				j++
			}
			if j >= n {
				// Unterminated "[": treat literally.
				sb.WriteString(`\[`)
				continue
			}
			stuff := pattern[i:j]
			stuff = strings.ReplaceAll(stuff, `\`, `\\`)
			i = j + 1
			if strings.HasPrefix(stuff, "!") {
				stuff = "^" + stuff[1:]
			} else if strings.HasPrefix(stuff, "^") {
				stuff = `\` + stuff
			}
			sb.WriteString("[" + stuff + "]")
		default:
			sb.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	return sb.String()
}

// nameRe matches runs of name characters: ASCII letters, digits, underscore
// and hyphen.
var nameRe = regexp.MustCompile(`[A-Za-z0-9_-]+`)

// getName extracts a package name from a string that may be a shell pattern
// or regular expression: the longest run of name characters, breaking ties
// by picking the first one found.
func getName(s string) string {
	matches := nameRe.FindAllString(s, -1)
	if len(matches) == 0 {
		return ""
	}
	best := matches[0]
	for _, m := range matches[1:] {
		if len(m) > len(best) {
			best = m
		}
	}
	return best
}

// productString normalizes product strings: openSUSE aliases are expanded,
// and SUSE Micro products are renamed based on their version.
//
// Malformed input (e.g. a non-numeric or missing version) is returned
// unchanged.
func productString(s string) string {
	switch s {
	case "Leap", "Leap_Micro", "Tumbleweed":
		return "openSUSE_" + s
	}
	if !strings.Contains(s, "Micro") || strings.Contains(s, "Leap") {
		return s
	}

	parts := strings.SplitN(s, "/", 2)
	if len(parts) < 2 || parts[1] == "" {
		return s
	}
	version := parts[1]

	major, err := strconv.Atoi(string(version[0]))
	if err != nil {
		return s
	}
	if major > 5 {
		return "SL-Micro/" + version
	}

	vparts := strings.SplitN(version, ".", 2)
	if len(vparts) < 2 {
		return s
	}
	sub, err := strconv.Atoi(vparts[1])
	if err != nil {
		return s
	}
	if sub > 2 {
		return "SLE-Micro/" + version
	}
	return "SUSE-MicroOS/" + version
}
