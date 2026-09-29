// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Base URLs, overridable in tests via httptest servers.
var (
	sccProductsURL  = "https://scc.suse.com/api/package_search/products"
	sccPackagesURL  = "https://scc.suse.com/api/package_search/packages"
	opensuseDistURL = "https://get.opensuse.org/api/v0/distributions.json"
	mirrorcacheURL  = "https://mirrorcache.opensuse.org/rest/search/package_locations"
)

// productPrefixes lists the SCC product identifier prefixes susepkg cares
// about.
var productPrefixes = []string{"SLES/", "SLE-Micro/", "SL-Micro/", "SUSE-MicroOS/"}

// eolProducts are products excluded because they are end-of-life.
var eolProducts = map[string]bool{
	"SLES/12":          true,
	"SLES/12.1":        true,
	"SLES/12.2":        true,
	"SLES/12.3":        true,
	"SLES/12.4":        true,
	"SLES/15":          true,
	"SLES/15.1":        true,
	"SLES/15.2":        true,
	"SLES/15.3":        true,
	"SUSE-MicroOS/5.0": true,
	"SUSE-MicroOS/5.1": true,
	"SUSE-MicroOS/5.2": true,
}

// product is a SUSE or openSUSE product/version identifier (e.g. "SLES/15.6",
// "openSUSE_Leap/15.6", "openSUSE_Tumbleweed"), plus the SCC product id needed
// to query package_search (0 for openSUSE products, which have none).
type product struct {
	name string
	id   int
	arch string
}

func (p product) String() string { return p.name }

func (p product) isOpenSUSE() bool { return strings.HasPrefix(p.name, "openSUSE") }

type sccProductEntry struct {
	ID           int    `json:"id"`
	Identifier   string `json:"identifier"`
	Architecture string `json:"architecture"`
}

type opensuseDistItem struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// fetchSUSEProducts fetches the full SCC product list.
func fetchSUSEProducts() ([]sccProductEntry, error) {
	var wrapper struct {
		Data []sccProductEntry `json:"data"`
	}
	headers := map[string]string{"Accept": "application/vnd.scc.suse.com.v4+json"}
	if err := getJSON(context.Background(), sccProductsURL, headers, nil, &wrapper); err != nil {
		return nil, err
	}
	return wrapper.Data, nil
}

// getSUSEProducts caches fetchSUSEProducts.
var getSUSEProducts = sync.OnceValues(fetchSUSEProducts)

// fetchOpenSUSEProducts fetches the stable openSUSE distributions (plus the
// latest Tumbleweed).
func fetchOpenSUSEProducts() ([]opensuseDistItem, error) {
	type distEntry struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		State   string `json:"state"`
	}
	headers := map[string]string{"Accept": "application/json"}
	var data map[string][]distEntry
	if err := getJSON(context.Background(), opensuseDistURL, headers, nil, &data); err != nil {
		return nil, err
	}

	var items []opensuseDistItem
	for key, list := range data {
		for i, item := range list {
			if item.State == "Stable" || (key == "Tumbleweed" && i == 0) {
				items = append(items, opensuseDistItem{
					Name:    strings.ReplaceAll(item.Name, " ", "_"),
					Version: item.Version,
				})
			}
		}
	}
	return items, nil
}

// getOpenSUSEProducts caches fetchOpenSUSEProducts.
var getOpenSUSEProducts = sync.OnceValues(fetchOpenSUSEProducts)

// getProducts returns the sorted list of products susepkg queries by
// default.
func getProducts(arch string) ([]product, error) {
	suseEntries, err := getSUSEProducts()
	if err != nil {
		return nil, err
	}

	var products []product
	for _, p := range suseEntries {
		if p.Architecture != arch {
			continue
		}
		if !hasAnyPrefix(p.Identifier, productPrefixes) {
			continue
		}
		name := strings.TrimSuffix(p.Identifier, "/"+arch)
		if eolProducts[name] {
			continue
		}
		products = append(products, product{name: name, id: p.ID, arch: arch})
	}
	sort.SliceStable(products, func(i, j int) bool {
		return productSortLess(products[i], products[j])
	})

	osEntries, err := getOpenSUSEProducts()
	if err != nil {
		return nil, err
	}
	osProducts := make([]product, 0, len(osEntries))
	for _, p := range osEntries {
		name := p.Name
		if name != "openSUSE_Tumbleweed" {
			name = fmt.Sprintf("%s/%s", p.Name, p.Version)
		}
		osProducts = append(osProducts, product{name: name, arch: arch})
	}
	slices.SortFunc(osProducts, func(a, b product) int {
		return strings.Compare(a.name, b.name)
	})

	return append(products, osProducts...), nil
}

func hasAnyPrefix(s string, prefixes []string) bool {
	return slices.ContainsFunc(prefixes, func(p string) bool { return strings.HasPrefix(s, p) })
}

// microVersionRe recognizes SUSE Micro products.
var microVersionRe = regexp.MustCompile(`^(SUSE-MicroOS|SLE-Micro|SL-Micro)/(\d+)\.(\d+)`)

var microOrder = map[string]int{"SUSE-MicroOS": 1, "SLE-Micro": 2, "SL-Micro": 3}

// productSortLess orders SLES products alphabetically first, followed by
// the Micro products grouped by family (SUSE-MicroOS, then SLE-Micro, then
// SL-Micro) and sorted by version.
func productSortLess(a, b product) bool {
	ma := microVersionRe.FindStringSubmatch(a.name)
	mb := microVersionRe.FindStringSubmatch(b.name)
	if ma == nil && mb == nil {
		return a.name < b.name
	}
	if ma == nil {
		return true
	}
	if mb == nil {
		return false
	}
	if oa, ob := microOrder[ma[1]], microOrder[mb[1]]; oa != ob {
		return oa < ob
	}
	if maja, majb := atoi(ma[2]), atoi(mb[2]); maja != majb {
		return maja < majb
	}
	return atoi(ma[3]) < atoi(mb[3])
}

// atoi parses a decimal string already validated by microVersionRe, so any
// error is impossible and safely ignored.
func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
