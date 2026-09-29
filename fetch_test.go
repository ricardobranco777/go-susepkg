// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"testing"
	"time"
)

func TestRsplitN(t *testing.T) {
	cases := []struct {
		s, sep string
		n      int
		want   []string
	}{
		{"a.b.c.d", ".", 2, []string{"a.b", "c", "d"}},
		{"a.b", ".", 2, []string{"a", "b"}},
		{"a", ".", 2, []string{"a"}},
		{"a-b-c", "-", 0, []string{"a-b-c"}},
		{"mariadb-client-11.6.1-1.1.x86_64.rpm", ".", 2, []string{"mariadb-client-11.6.1-1.1", "x86_64", "rpm"}},
	}
	for _, c := range cases {
		got := rsplitN(c.s, c.sep, c.n)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("rsplitN(%q, %q, %d) = %v, want %v", c.s, c.sep, c.n, got, c.want)
		}
	}
}

func TestParseOpenSUSEFilename(t *testing.T) {
	cases := []struct {
		file                   string
		name, version, release string
		ok                     bool
	}{
		{
			file: "mariadb-client-11.6.1-1.1.x86_64.rpm",
			name: "mariadb-client", version: "11.6.1", release: "1.1",
			ok: true,
		},
		{
			file: "python3-foo-1.0-1.noarch.rpm",
			name: "python3-foo", version: "1.0", release: "1",
			ok: true,
		},
		{
			file: "podman-5.2.2-1.1.x86_64.rpm",
			name: "podman", version: "5.2.2", release: "1.1",
			ok: true,
		},
		// Fewer than two dashes: not enough fields for name/version/release.
		{file: "noversionhere.rpm", ok: false},
		{file: "foo-bar.rpm", ok: false},
		// Two dashes still parses even with non-numeric fields: the content of
		// the fields isn't validated.
		{
			file: "no-version-here.rpm",
			name: "no", version: "version", release: "here",
			ok: true,
		},
	}
	for _, c := range cases {
		name, version, release, ok := parseOpenSUSEFilename(c.file)
		if ok != c.ok {
			t.Errorf("parseOpenSUSEFilename(%q) ok = %v, want %v", c.file, ok, c.ok)
			continue
		}
		if !ok {
			continue
		}
		if name != c.name || version != c.version || release != c.release {
			t.Errorf("parseOpenSUSEFilename(%q) = (%q, %q, %q), want (%q, %q, %q)",
				c.file, name, version, release, c.name, c.version, c.release)
		}
	}
}

func TestFetchVersionSCC(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("product_id"); got != "42" {
			t.Errorf("product_id = %q, want 42", got)
		}
		if got := r.URL.Query().Get("query"); got != "podman" {
			t.Errorf("query = %q, want podman", got)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"name": "podman", "version": "5.2.2", "release": "1.1", "products": []map[string]string{{"name": "SLES"}}},
				{"name": "podman", "version": "5.1.0", "release": "1.1", "products": []map[string]string{{"name": "SLES"}}},
				{"name": "podman-remote", "version": "5.2.2", "release": "1.1", "products": []map[string]string{{"name": "SLES"}}},
				{"name": "podman-suse", "version": "9.9.9", "release": "1", "products": []map[string]string{{"name": "SUSE Package Hub"}}},
			},
		})
	}))
	defer srv.Close()
	restore := sccPackagesURL
	sccPackagesURL = srv.URL
	defer func() { sccPackagesURL = restore }()

	re, err := getRegex("podman", false, false)
	if err != nil {
		t.Fatal(err)
	}
	p := product{name: "SLES/15.6", id: 42, arch: "x86_64"}
	packages, err := fetchVersion(context.Background(), p, "podman", re)
	if err != nil {
		t.Fatal(err)
	}
	if len(packages) != 1 {
		t.Fatalf("got %d packages, want 1: %+v", len(packages), packages)
	}
	got := packages[0]
	if got.name != "podman" || got.product != "SLES/15.6" || got.version.String() != "5.2.2-1.1" {
		t.Errorf("got %+v", got)
	}
}

func TestFetchVersionOpenSUSE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if got := q.Get("os"); got != "tumbleweed" {
			t.Errorf("os = %q, want tumbleweed", got)
		}
		if got := q.Get("official"); got != "1" {
			t.Errorf("official = %q, want 1", got)
		}
		if q.Has("os_ver") {
			t.Errorf("os_ver should not be set for Tumbleweed, got %q", q.Get("os_ver"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{
				{"file": "podman-5.2.2-1.1.x86_64.rpm"},
				{"file": "podman-5.1.0-1.1.x86_64.rpm"},
				{"file": "podman-remote-5.2.2-1.1.x86_64.rpm"},
			},
		})
	}))
	defer srv.Close()
	restore := mirrorcacheURL
	mirrorcacheURL = srv.URL
	defer func() { mirrorcacheURL = restore }()

	re, err := getRegex("podman", false, false)
	if err != nil {
		t.Fatal(err)
	}
	p := product{name: "openSUSE_Tumbleweed", arch: "x86_64"}
	packages, err := fetchVersion(context.Background(), p, "podman", re)
	if err != nil {
		t.Fatal(err)
	}
	if len(packages) != 1 {
		t.Fatalf("got %d packages, want 1: %+v", len(packages), packages)
	}
	got := packages[0]
	if got.name != "podman" || got.product != "Tumbleweed" || got.version.String() != "5.2.2-1.1" {
		t.Errorf("got %+v", got)
	}
}

func TestFetchVersionOpenSUSEWithVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if got := q.Get("os"); got != "leap-micro" {
			t.Errorf("os = %q, want leap-micro", got)
		}
		if got := q.Get("os_ver"); got != "6.0" {
			t.Errorf("os_ver = %q, want 6.0", got)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{}})
	}))
	defer srv.Close()
	restore := mirrorcacheURL
	mirrorcacheURL = srv.URL
	defer func() { mirrorcacheURL = restore }()

	re, err := getRegex("podman", false, false)
	if err != nil {
		t.Fatal(err)
	}
	p := product{name: "openSUSE_Leap_Micro/6.0", arch: "x86_64"}
	if _, err := fetchVersion(context.Background(), p, "podman", re); err != nil {
		t.Fatal(err)
	}
}

func TestFetchVersionErrorPropagates(t *testing.T) {
	defer func(d time.Duration) { retryDelay = d }(retryDelay)
	retryDelay = time.Millisecond
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	restore := sccPackagesURL
	sccPackagesURL = srv.URL
	defer func() { sccPackagesURL = restore }()

	re := regexp.MustCompile(".*")
	p := product{name: "SLES/15.6", id: 1, arch: "x86_64"}
	if _, err := fetchVersion(context.Background(), p, "podman", re); err == nil {
		t.Error("expected an error for HTTP 500")
	}
}
