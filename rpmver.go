// SPDX-License-Identifier: BSD-2-Clause

package main

import "strings"

// rpmvercmp compares two RPM version (or release) strings, matching the
// behavior of librpm's rpmvercmp().
//
// It returns -1, 0 or 1 the same way strings.Compare does.
func rpmvercmp(a, b string) int {
	if a == b {
		return 0
	}

	one, two := a, b

	for len(one) > 0 || len(two) > 0 {
		one = skipSeparators(one)
		two = skipSeparators(two)

		// The tilde separator sorts before everything else, including the
		// empty string.
		if hasPrefixByte(one, '~') || hasPrefixByte(two, '~') {
			if !hasPrefixByte(one, '~') {
				return 1
			}
			if !hasPrefixByte(two, '~') {
				return -1
			}
			one = one[1:]
			two = two[1:]
			continue
		}

		// The caret separator sorts before the tail but after the empty
		// string.
		if hasPrefixByte(one, '^') || hasPrefixByte(two, '^') {
			if one == "" {
				return -1
			}
			if two == "" {
				return 1
			}
			if !hasPrefixByte(one, '^') {
				return 1
			}
			if !hasPrefixByte(two, '^') {
				return -1
			}
			one = one[1:]
			two = two[1:]
			continue
		}

		if one == "" || two == "" {
			break
		}

		var segOne, segTwo string
		var isNum bool
		if isDigit(one[0]) {
			segOne, one = splitFunc(one, isDigit)
			segTwo, two = splitFunc(two, isDigit)
			isNum = true
		} else {
			segOne, one = splitFunc(one, isAlpha)
			segTwo, two = splitFunc(two, isAlpha)
			isNum = false
		}

		// Different segment types: numeric segments are always newer than
		// alpha segments (including the empty "no segment" case).
		if segTwo == "" {
			if isNum {
				return 1
			}
			return -1
		}

		if isNum {
			segOne = strings.TrimLeft(segOne, "0")
			segTwo = strings.TrimLeft(segTwo, "0")
			if len(segOne) > len(segTwo) {
				return 1
			}
			if len(segTwo) > len(segOne) {
				return -1
			}
		}

		if segOne < segTwo {
			return -1
		}
		if segOne > segTwo {
			return 1
		}
	}

	if one == "" && two == "" {
		return 0
	}
	if one == "" {
		return -1
	}
	return 1
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func isAlpha(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func isAlphaNum(b byte) bool {
	return isDigit(b) || isAlpha(b)
}

// skipSeparators drops leading bytes that are neither ASCII alphanumeric nor
// the '~' and '^' markers. It works on bytes, not runes, like librpm does, so
// non-ASCII characters are always separators.
func skipSeparators(s string) string {
	i := 0
	for i < len(s) && !isAlphaNum(s[i]) && s[i] != '~' && s[i] != '^' {
		i++
	}
	return s[i:]
}

func hasPrefixByte(s string, b byte) bool {
	return len(s) > 0 && s[0] == b
}

// splitFunc consumes the longest prefix of s made of bytes matching pred and
// returns (prefix, remainder).
func splitFunc(s string, pred func(byte) bool) (string, string) {
	i := 0
	for i < len(s) && pred(s[i]) {
		i++
	}
	return s[:i], s[i:]
}

// rpmVersion holds a package's version and release.
type rpmVersion struct {
	version string
	release string
}

func (v rpmVersion) String() string {
	return v.version + "-" + v.release
}

// less reports whether v sorts strictly before other, treating version as
// the primary key and release as the tiebreaker (the epoch is always treated
// as equal).
func (v rpmVersion) less(other rpmVersion) bool {
	if c := rpmvercmp(v.version, other.version); c != 0 {
		return c < 0
	}
	return rpmvercmp(v.release, other.release) < 0
}
