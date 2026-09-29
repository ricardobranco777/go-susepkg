// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// resetProductCaches clears the sync.OnceValues memoization between tests,
// since getSUSEProducts/getOpenSUSEProducts are package-level singletons.
func resetProductCaches() {
	getSUSEProducts = sync.OnceValues(fetchSUSEProducts)
	getOpenSUSEProducts = sync.OnceValues(fetchOpenSUSEProducts)
}

func TestGetProducts(t *testing.T) {
	sccSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": []sccProductEntry{
				{ID: 1, Identifier: "SLES/15.6/x86_64", Architecture: "x86_64"},
				{ID: 2, Identifier: "SLES/15.3/x86_64", Architecture: "x86_64"}, // EOL, excluded
				{ID: 3, Identifier: "SLES/16.0/x86_64", Architecture: "x86_64"},
				{ID: 4, Identifier: "SLES/15.6/aarch64", Architecture: "aarch64"}, // wrong arch
				{ID: 5, Identifier: "SLE-Micro/5.5/x86_64", Architecture: "x86_64"},
				{ID: 6, Identifier: "SL-Micro/6.0/x86_64", Architecture: "x86_64"},
				{ID: 7, Identifier: "SUSE-MicroOS/5.9/x86_64", Architecture: "x86_64"},   // not a real version, just not EOL
				{ID: 8, Identifier: "openSUSE-Leap/15.6/x86_64", Architecture: "x86_64"}, // wrong prefix
			},
		})
	}))
	defer sccSrv.Close()

	distSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"Tumbleweed": []map[string]any{
				{"name": "openSUSE_Tumbleweed", "version": "", "state": "Unstable"},
				{"name": "openSUSE_Tumbleweed", "version": "old", "state": "Unstable"},
			},
			"Leap": []map[string]any{
				{"name": "openSUSE Leap", "version": "15.6", "state": "Stable"},
			},
			"Leap Micro": []map[string]any{
				{"name": "openSUSE Leap Micro", "version": "6.0", "state": "Stable"},
				{"name": "openSUSE Leap Micro", "version": "6.1", "state": "Beta"},
			},
		})
	}))
	defer distSrv.Close()

	restoreSCC, restoreDist := sccProductsURL, opensuseDistURL
	sccProductsURL, opensuseDistURL = sccSrv.URL, distSrv.URL
	resetProductCaches()
	defer func() {
		sccProductsURL, opensuseDistURL = restoreSCC, restoreDist
		resetProductCaches()
	}()

	products, err := getProducts("x86_64")
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, p := range products {
		names = append(names, p.name)
	}
	want := []string{
		"SLES/15.6",
		"SLES/16.0",
		"SUSE-MicroOS/5.9",
		"SLE-Micro/5.5",
		"SL-Micro/6.0",
		"openSUSE_Leap/15.6",
		"openSUSE_Leap_Micro/6.0",
		"openSUSE_Tumbleweed",
	}
	if len(names) != len(want) {
		t.Fatalf("got products %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("products[%d] = %q, want %q (full: %v)", i, names[i], want[i], names)
		}
	}

	for _, p := range products {
		if p.name == "SLES/15.6" && p.id != 1 {
			t.Errorf("SLES/15.6 id = %d, want 1", p.id)
		}
	}
}

func TestProductSortLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"SLES/15.6", "SLES/16.0", true},
		{"SLES/16.0", "SLES/15.6", false},
		{"SLES/15.6", "SUSE-MicroOS/5.1", true}, // SLES always before Micro group
		{"SUSE-MicroOS/5.9", "SLE-Micro/5.1", true},
		{"SLE-Micro/5.9", "SL-Micro/6.0", true},
		{"SL-Micro/6.0", "SL-Micro/6.1", true},
		{"SL-Micro/6.10", "SL-Micro/6.9", false}, // numeric, not lexical
	}
	for _, c := range cases {
		a, b := product{name: c.a}, product{name: c.b}
		if got := productSortLess(a, b); got != c.want {
			t.Errorf("productSortLess(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
