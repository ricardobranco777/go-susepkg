// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"context"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// pkg is one resolved package: a name, its highest known RPM version, and
// the product it was found in.
type pkg struct {
	name    string
	product string
	version rpmVersion
}

type sccPackageEntry struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Release  string `json:"release"`
	Products []struct {
		Name string `json:"name"`
	} `json:"products"`
}

type mirrorcachePackageLocation struct {
	File string `json:"file"`
}

// fetchVersion fetches, for the given product, the latest version of every
// package whose name matches re.
func fetchVersion(ctx context.Context, p product, name string, re *regexp.Regexp) ([]pkg, error) {
	type nameVer struct {
		name    string
		version rpmVersion
	}
	var entries []nameVer

	if p.isOpenSUSE() {
		params := url.Values{
			"ignore_file": {"json"},
			"ignore_path": {"/repositories/home:"},
			"official":    {"1"},
			"package":     {name},
		}
		parts := strings.SplitN(p.name, "/", 2)
		osName := strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(parts[0], "openSUSE_"), "_", "-"))
		params.Set("os", osName)
		if len(parts) == 2 {
			params.Set("os_ver", parts[1])
		}

		var wrapper struct {
			Data []mirrorcachePackageLocation `json:"data"`
		}
		headers := map[string]string{"Accept": "application/json"}
		if err := getJSON(ctx, mirrorcacheURL, headers, params, &wrapper); err != nil {
			return nil, err
		}
		for _, loc := range wrapper.Data {
			pname, version, release, ok := parseOpenSUSEFilename(loc.File)
			if !ok || !re.MatchString(pname) {
				continue
			}
			entries = append(entries, nameVer{name: pname, version: rpmVersion{version: version, release: release}})
		}
	} else {
		params := url.Values{
			"query":      {name},
			"product_id": {strconv.Itoa(p.id)},
		}
		var wrapper struct {
			Data []sccPackageEntry `json:"data"`
		}
		headers := map[string]string{"Accept": "application/vnd.scc.suse.com.v4+json"}
		if err := getJSON(ctx, sccPackagesURL, headers, params, &wrapper); err != nil {
			return nil, err
		}
		for _, info := range wrapper.Data {
			if len(info.Products) > 0 && info.Products[0].Name == "SUSE Package Hub" {
				continue
			}
			if !re.MatchString(info.Name) {
				continue
			}
			entries = append(entries, nameVer{name: info.Name, version: rpmVersion{version: info.Version, release: info.Release}})
		}
	}

	latest := make(map[string]rpmVersion, len(entries))
	for _, e := range entries {
		if cur, ok := latest[e.name]; !ok || cur.less(e.version) {
			latest[e.name] = e.version
		}
	}
	names := slices.Sorted(maps.Keys(latest))

	displayProduct := strings.TrimPrefix(p.name, "openSUSE_")
	packages := make([]pkg, 0, len(names))
	for _, n := range names {
		packages = append(packages, pkg{name: n, product: displayProduct, version: latest[n]})
	}
	return packages, nil
}

// parseOpenSUSEFilename extracts a package's name, version and release from
// an RPM filename such as "mariadb-client-11.6.1-1.1.x86_64.rpm". It reports
// ok=false for anything that doesn't fit the "name-version-release.arch.rpm"
// layout.
func parseOpenSUSEFilename(file string) (name, version, release string, ok bool) {
	// Drop the last two dot-separated fields (typically "arch.rpm").
	base := rsplitN(file, ".", 2)[0]

	// The last two dash-separated fields of what remains are the version
	// and release.
	parts := rsplitN(base, "-", 2)
	if len(parts) < 3 {
		return "", "", "", false
	}
	version, release = parts[1], parts[2]

	idx := strings.Index(file, "-"+version+"-"+release)
	if idx < 0 {
		return "", "", "", false
	}
	return file[:idx], version, release, true
}

// rsplitN splits s on sep at most n times, starting from the right, so the
// first element of the result may itself still contain sep.
func rsplitN(s, sep string, n int) []string {
	var tail []string
	for ; n > 0; n-- {
		i := strings.LastIndex(s, sep)
		if i < 0 {
			break
		}
		tail = append(tail, s[i+len(sep):])
		s = s[:i]
	}
	parts := []string{s}
	for _, t := range slices.Backward(tail) {
		parts = append(parts, t)
	}
	return parts
}
