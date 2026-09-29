// SPDX-License-Identifier: BSD-2-Clause

package main

import "testing"

// Test vectors from rpm's own test suite (tests/rpmvercmp.at), used to make
// sure rpmvercmp() matches librpm's rpmvercmp() exactly.
func TestRpmvercmp(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0", "1.0", 0},
		{"1.0", "2.0", -1},
		{"2.0", "1.0", 1},
		{"2.0.1", "2.0.1", 0},
		{"2.0", "2.0.1", -1},
		{"2.0.1", "2.0", 1},
		{"2.0.1a", "2.0.1a", 0},
		{"2.0.1a", "2.0.1", 1},
		{"2.0.1", "2.0.1a", -1},
		{"5.5p1", "5.5p1", 0},
		{"5.5p1", "5.5p2", -1},
		{"5.5p2", "5.5p1", 1},
		{"5.5p10", "5.5p10", 0},
		{"5.5p1", "5.5p10", -1},
		{"5.5p10", "5.5p1", 1},
		{"10xyz", "10.1xyz", -1},
		{"10.1xyz", "10xyz", 1},
		{"xyz10", "xyz10", 0},
		{"xyz10", "xyz10.1", -1},
		{"xyz10.1", "xyz10", 1},
		{"xyz.4", "xyz.4", 0},
		{"xyz.4", "8", -1},
		{"8", "xyz.4", 1},
		{"xyz.4", "2", -1},
		{"2", "xyz.4", 1},
		{"5.5p2", "5.6p1", -1},
		{"5.6p1", "5.5p2", 1},
		{"5.6p1", "6.5p1", -1},
		{"6.5p1", "5.6p1", 1},
		{"6.0.rc1", "6.0", 1},
		{"6.0", "6.0.rc1", -1},
		{"10b2", "10a1", 1},
		{"10a2", "10b2", -1},
		{"1.0aa", "1.0aa", 0},
		{"1.0a", "1.0aa", -1},
		{"1.0aa", "1.0a", 1},
		{"10.0001", "10.0001", 0},
		{"10.0001", "10.1", 0},
		{"10.1", "10.0001", 0},
		{"10.0001", "10.0039", -1},
		{"10.0039", "10.0001", 1},
		{"4.999.9", "5.0", -1},
		{"5.0", "4.999.9", 1},
		{"20101121", "20101121", 0},
		{"20101121", "20101122", -1},
		{"20101122", "20101121", 1},
		{"2_0", "2_0", 0},
		{"2.0", "2_0", 0},
		{"2_0", "2.0", 0},
		{"a", "a", 0},
		{"a+", "a+", 0},
		{"a+", "a_", 0},
		{"a_", "a+", 0},
		{"+a", "+a", 0},
		{"+a", "_a", 0},
		{"_a", "+a", 0},
		{"+_", "+_", 0},
		{"_+", "+_", 0},
		{"_+", "_+", 0},
		{"+", "+", 0},
		{"+", "_", 0},
		{"_", "+", 0},
		{"_", "_", 0},
		{"1.0~rc1", "1.0~rc1", 0},
		{"1.0~rc1", "1.0", -1},
		{"1.0", "1.0~rc1", 1},
		{"1.0~rc1", "1.0~rc2", -1},
		{"1.0~rc2", "1.0~rc1", 1},
		{"1.0~rc1~git123", "1.0~rc1~git123", 0},
		{"1.0~rc1~git123", "1.0~rc1", -1},
		{"1.0~rc1", "1.0~rc1~git123", 1},
		{"1.0^", "1.0^", 0},
		{"1.0^subrel1", "1.0^subrel1", 0},
		{"1.0^subrel1", "1.0", 1},
		{"1.0", "1.0^subrel1", -1},
		{"1.0^subrel1", "1.0^subrel2", -1},
		{"1.0^subrel2", "1.0^subrel1", 1},
		{"1.0~rc1^git1", "1.0~rc1^git1", 0},
		{"1.0~rc1^git1", "1.0~rc1", 1},
		{"1.0~rc1", "1.0~rc1^git1", -1},
		// Non-ASCII characters are separators, like any other non-alphanumeric
		// byte. U+0141 must not be mistaken for 'A' (its low byte is 0x41).
		{"\u0141", "", 0},
		{"1\u01411", "1.1", 0},
		{"\u0141", "_\u0141+\xc5\u00e9\xff", 0},
		{"1.0\u00e9", "1.0", 0},
		{"1.0^git1~pre", "1.0^git1~pre", 0},
		{"1.0^git1", "1.0^git1~pre", 1},
		{"1.0^git1~pre", "1.0^git1", -1},
	}

	for _, c := range cases {
		if got := rpmvercmp(c.a, c.b); got != c.want {
			t.Errorf("rpmvercmp(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
		// rpmvercmp must be antisymmetric.
		if got := rpmvercmp(c.b, c.a); got != -c.want {
			t.Errorf("rpmvercmp(%q, %q) = %d, want %d", c.b, c.a, got, -c.want)
		}
	}
}

func TestRPMVersionLess(t *testing.T) {
	cases := []struct {
		a, b rpmVersion
		want bool
	}{
		{rpmVersion{"1.0", "1"}, rpmVersion{"1.1", "1"}, true},
		{rpmVersion{"1.1", "1"}, rpmVersion{"1.0", "1"}, false},
		{rpmVersion{"1.0", "1"}, rpmVersion{"1.0", "2"}, true},
		{rpmVersion{"1.0", "2"}, rpmVersion{"1.0", "1"}, false},
		{rpmVersion{"1.0", "1"}, rpmVersion{"1.0", "1"}, false},
	}
	for _, c := range cases {
		if got := c.a.less(c.b); got != c.want {
			t.Errorf("%v.less(%v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestRPMVersionString(t *testing.T) {
	v := rpmVersion{version: "1.2.3", release: "4.1"}
	if got, want := v.String(), "1.2.3-4.1"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
