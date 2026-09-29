// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSelectProducts(t *testing.T) {
	all := []product{
		{name: "SLES/15.6"}, {name: "SLES/15.7"}, {name: "SL-Micro/6.0"},
		{name: "openSUSE_Leap/15.6"}, {name: "openSUSE_Tumbleweed"},
	}
	names := func(ps []product) string {
		var s []string
		for _, p := range ps {
			s = append(s, p.name)
		}
		return strings.Join(s, ",")
	}
	cases := []struct {
		req     []string
		want    string
		wantErr bool
	}{
		{nil, "SLES/15.6,SLES/15.7,SL-Micro/6.0,openSUSE_Leap/15.6,openSUSE_Tumbleweed", false},
		{[]string{"SLES/15.6"}, "SLES/15.6", false},
		{[]string{"sles"}, "SLES/15.6,SLES/15.7", false},
		{[]string{"weed"}, "openSUSE_Tumbleweed", false},
		{[]string{"SLES/15.6", "Micro"}, "SLES/15.6,SL-Micro/6.0", false},
		{[]string{"nope"}, "", true},
	}
	for _, c := range cases {
		got, err := selectProducts(all, c.req)
		if (err != nil) != c.wantErr {
			t.Errorf("selectProducts(%v) err = %v", c.req, err)
			continue
		}
		if !c.wantErr && names(got) != c.want {
			t.Errorf("selectProducts(%v) = %s, want %s", c.req, names(got), c.want)
		}
	}
}

func TestPrintPackages(t *testing.T) {
	var buf bytes.Buffer
	printPackages(&buf, []pkg{
		{name: "podman", product: "SLES/15.6", version: rpmVersion{"5.2.2", "1.1"}},
		{name: "podman-remote", product: "Tumbleweed", version: rpmVersion{"5.3.0", "2"}},
	})
	want := "SLES/15.6   podman         5.2.2-1.1\n" +
		"Tumbleweed  podman-remote  5.3.0-2\n"
	if buf.String() != want {
		t.Errorf("got:\n%q\nwant:\n%q", buf.String(), want)
	}
}

func TestRunEndToEnd(t *testing.T) {
	scc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/products") || r.URL.Query().Get("product_id") == "" {
			json.NewEncoder(w).Encode(map[string]any{"data": []sccProductEntry{
				{ID: 1, Identifier: "SLES/15.6/x86_64", Architecture: "x86_64"},
				{ID: 2, Identifier: "SLES/15.7/x86_64", Architecture: "x86_64"},
			}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
			{"name": "podman", "version": "5.2.2", "release": "1." + r.URL.Query().Get("product_id"),
				"products": []map[string]string{{"name": "SLES"}}},
		}})
	}))
	defer scc.Close()
	dist := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer dist.Close()

	rs, rp, rd := sccProductsURL, sccPackagesURL, opensuseDistURL
	sccProductsURL, sccPackagesURL, opensuseDistURL = scc.URL+"/products", scc.URL+"/packages", dist.URL
	resetProductCaches()
	defer func() {
		sccProductsURL, sccPackagesURL, opensuseDistURL = rs, rp, rd
		resetProductCaches()
	}()

	var out, errb bytes.Buffer
	code := run(context.Background(), []string{"-a", "x86_64", "podman", "-p", "SLES"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	want := "SLES/15.6  podman  5.2.2-1.1\nSLES/15.7  podman  5.2.2-1.2\n"
	if out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}

	out.Reset()
	if code := run(context.Background(), []string{"-p", "list"}, &out, &errb); code != 0 {
		t.Fatalf("list exit %d", code)
	}
	if out.String() != "SLES/15.6\nSLES/15.7\n" {
		t.Errorf("list output %q", out.String())
	}
}

func TestRunErrors(t *testing.T) {
	cases := []struct {
		args []string
		code int
	}{
		{[]string{"podman"}, 2},                       // -p is required
		{[]string{"-p", "any"}, 1},                    // no package
		{[]string{"-p", "any", "-a", "mips", "x"}, 2}, // bad arch
		{[]string{"-p", "any", "-x", "(", "x"}, 1},    // bad regex
		{[]string{"-p", "any", "***"}, 1},             // no usable name
		{[]string{"--bogus"}, 2},
	}
	for _, c := range cases {
		var out, errb bytes.Buffer
		if got := run(context.Background(), c.args, &out, &errb); got != c.code {
			t.Errorf("run(%v) = %d, want %d (stderr: %s)", c.args, got, c.code, errb.String())
		}
	}
}

func TestRunVersion(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"--version"}, &out, &errb); code != 0 || out.String() != "v"+version+"\n" {
		t.Errorf("got code %d out %q", code, out.String())
	}
}

func TestRunFetchFailureExitCode(t *testing.T) {
	defer func(d time.Duration) { retryDelay = d }(retryDelay)
	retryDelay = time.Millisecond

	scc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/products") {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
				{"id": 1, "identifier": "SLES/15.6/x86_64", "architecture": "x86_64"},
			}})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer scc.Close()
	dist := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{}"))
	}))
	defer dist.Close()

	rs, rp, rd := sccProductsURL, sccPackagesURL, opensuseDistURL
	sccProductsURL, sccPackagesURL, opensuseDistURL = scc.URL+"/products", scc.URL+"/packages", dist.URL
	resetProductCaches()
	defer func() {
		sccProductsURL, sccPackagesURL, opensuseDistURL = rs, rp, rd
		resetProductCaches()
	}()

	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"-a", "x86_64", "-p", "SLES", "podman"}, &out, &errb); code != 1 {
		t.Errorf("exit %d, want 1 (stderr: %s)", code, errb.String())
	}
}
