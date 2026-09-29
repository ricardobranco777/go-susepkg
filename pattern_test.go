// SPDX-License-Identifier: BSD-2-Clause

package main

import "testing"

func TestGetRegexPlain(t *testing.T) {
	re, err := getRegex("podman", false, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		s    string
		want bool
	}{
		{"podman", true},
		{"podman-remote", false}, // anchored at the end: must match "podman$"
		{"libpodman", false},     // anchored at the start too
		{"Podman", false},
	} {
		if got := re.MatchString(tc.s); got != tc.want {
			t.Errorf("MatchString(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

func TestGetRegexInsensitive(t *testing.T) {
	re, err := getRegex("podman", true, false)
	if err != nil {
		t.Fatal(err)
	}
	if !re.MatchString("Podman") {
		t.Error("expected case-insensitive match")
	}
}

func TestGetRegexRegex(t *testing.T) {
	re, err := getRegex("podman-.*", false, true)
	if err != nil {
		t.Fatal(err)
	}
	if !re.MatchString("podman-remote") {
		t.Error("expected regex match")
	}
	if !re.MatchString("podman-remote-extra") {
		t.Error("expected regex match without end anchor")
	}
	if re.MatchString("xpodman-remote") {
		t.Error("expected no match: not anchored at start")
	}
}

func TestGetRegexGlob(t *testing.T) {
	cases := []struct {
		pattern string
		s       string
		want    bool
	}{
		{"*podman*", "libpodman-remote", true},
		{"podman-?", "podman-1", true},
		{"podman-?", "podman-10", false},
		{"podman[0-9]", "podman5", true},
		{"podman[0-9]", "podmana", false},
		{"podman[!0-9]", "podmana", true},
		{"podman[!0-9]", "podman5", false},
		{"*podman*", "podman", true},
		{"podman*", "xpodman", false}, // start anchor still applies
	}
	for _, c := range cases {
		re, err := getRegex(c.pattern, false, false)
		if err != nil {
			t.Fatalf("getRegex(%q): %v", c.pattern, err)
		}
		if got := re.MatchString(c.s); got != c.want {
			t.Errorf("pattern %q MatchString(%q) = %v, want %v", c.pattern, c.s, got, c.want)
		}
	}
}

func TestGetName(t *testing.T) {
	cases := []struct {
		s    string
		want string
	}{
		{"podman", "podman"},
		{"*podman*", "podman"},
		{"podman-.*", "podman-"},
		{"^podman$", "podman"},
		{"(ardvark-dns|buildah)", "ardvark-dns"},
		{"", ""},
		{"***", ""},
	}
	for _, c := range cases {
		if got := getName(c.s); got != c.want {
			t.Errorf("getName(%q) = %q, want %q", c.s, got, c.want)
		}
	}
}

func TestProductString(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Leap", "openSUSE_Leap"},
		{"Leap_Micro", "openSUSE_Leap_Micro"},
		{"Tumbleweed", "openSUSE_Tumbleweed"},
		{"SLES/15.6", "SLES/15.6"},
		{"openSUSE_Leap/15.6", "openSUSE_Leap/15.6"},
		{"Micro/6.0", "SL-Micro/6.0"},
		{"Micro/6.1", "SL-Micro/6.1"},
		{"Micro/5.3", "SLE-Micro/5.3"},
		{"Micro/5.4", "SLE-Micro/5.4"},
		{"Micro/5.1", "SUSE-MicroOS/5.1"},
		{"Micro/5.2", "SUSE-MicroOS/5.2"},
		// Malformed input is returned unchanged.
		{"Micro", "Micro"},
		{"Micro/", "Micro/"},
		{"Micro/x.0", "Micro/x.0"},
		{"Micro/5", "Micro/5"},
	}
	for _, c := range cases {
		if got := productString(c.in); got != c.want {
			t.Errorf("productString(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
