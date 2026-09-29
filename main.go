// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"

	"github.com/spf13/pflag"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

const maxWorkers = 10

var validArchs = []string{"aarch64", "ppc64le", "s390x", "x86_64"}

func defaultArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	}
	return runtime.GOARCH
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// run is the testable body of main. It returns the process exit code.
func run(ctx context.Context, argv []string, stdout, stderr io.Writer) int {
	fs := pflag.NewFlagSet("susepkg", pflag.ContinueOnError)
	fs.SetOutput(stderr)
	arch := fs.StringP("arch", "a", defaultArch(), "architecture: "+strings.Join(validArchs, ", "))
	insensitive := fs.BoolP("insensitive", "i", false, "case insensitive search")
	prods := fs.StringArrayP("product", "p", nil, "product or 'list' or 'any'. May be specified multiple times")
	isRegex := fs.BoolP("regex", "x", false, "search regular expression")
	showVersion := fs.Bool("version", false, "show program's version number and exit")
	help := fs.BoolP("help", "h", false, "show this help message and exit")

	usage := func(w io.Writer) {
		fmt.Fprintln(w, "usage: susepkg [-h] [-a ARCH] [-i] -p PRODUCT [-x] [--version] [package]")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "show SUSE package versions")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "positional arguments:")
		fmt.Fprintln(w, "  package  may be a shell pattern or regular expression")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "options:")
		fmt.Fprint(w, fs.FlagUsages())
	}
	fs.Usage = func() { usage(stderr) }

	if err := fs.Parse(argv); err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			usage(stdout)
			return 0
		}
		return 2
	}
	if *help {
		usage(stdout)
		return 0
	}
	if *showVersion {
		fmt.Fprintf(stdout, "v%s\n", version)
		return 0
	}
	if !slices.Contains(validArchs, *arch) {
		fmt.Fprintf(stderr, "susepkg: error: argument -a/--arch: invalid choice: %q (choose from %s)\n",
			*arch, strings.Join(validArchs, ", "))
		return 2
	}
	if len(*prods) == 0 {
		usage(stderr)
		fmt.Fprintln(stderr, "susepkg: error: the following arguments are required: -p/--product")
		return 2
	}
	requested := make([]string, len(*prods))
	for i, p := range *prods {
		requested[i] = productString(p)
	}

	if len(requested) == 1 && requested[0] == "list" {
		all, err := getProducts(*arch)
		if err != nil {
			logError(stderr, err)
			return 1
		}
		for _, p := range all {
			fmt.Fprintln(stdout, p)
		}
		return 0
	}
	if fs.NArg() == 0 || fs.Arg(0) == "" {
		usage(stderr)
		return 1
	}
	pattern := fs.Arg(0)

	if len(requested) == 1 && requested[0] == "any" {
		requested = nil
	}
	re, err := getRegex(pattern, *insensitive, *isRegex)
	if err != nil {
		fmt.Fprintf(stderr, "susepkg: invalid pattern %q: %v\n", pattern, err)
		return 1
	}
	name := getName(pattern)
	if name == "" {
		fmt.Fprintf(stderr, "Invalid package: %s\n", pattern)
		return 1
	}

	all, err := getProducts(*arch)
	if err != nil {
		logError(stderr, err)
		return 1
	}
	products, err := selectProducts(all, requested)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	packages, failed := fetchAll(ctx, products, name, re, stderr)
	if ctx.Err() != nil {
		return 1
	}
	printPackages(stdout, packages)
	if failed > 0 {
		return 1
	}
	return 0
}

// selectProducts resolves the requested product names against the known
// products: an exact match first, then a case-insensitive substring match.
// An empty request selects everything.
func selectProducts(all []product, requested []string) ([]product, error) {
	if len(requested) == 0 {
		return all, nil
	}
	var selected []product
	for _, abbrev := range requested {
		var matched []product
		for _, p := range all {
			if p.name == abbrev {
				matched = append(matched, p)
			}
		}
		if len(matched) == 0 {
			lower := strings.ToLower(abbrev)
			for _, p := range all {
				if strings.Contains(strings.ToLower(p.name), lower) {
					matched = append(matched, p)
				}
			}
		}
		if len(matched) == 0 {
			return nil, fmt.Errorf("no matching product for: %s", abbrev)
		}
		selected = append(selected, matched...)
	}
	return selected, nil
}

// fetchAll queries every product concurrently (bounded by maxWorkers) and
// returns the packages in product order along with the number of products
// that failed. Failed products are reported on stderr and skipped.
func fetchAll(ctx context.Context, products []product, name string, re *regexp.Regexp, stderr io.Writer) ([]pkg, int) {
	results := make([][]pkg, len(products))
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup
	var mu sync.Mutex // serializes stderr output and protects failed
	var failed int
	for i, p := range products {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			pkgs, err := fetchVersion(ctx, p, name, re)
			if err != nil {
				if ctx.Err() == nil {
					mu.Lock()
					failed++
					logError(stderr, err)
					mu.Unlock()
				}
				return
			}
			results[i] = pkgs
		}()
	}
	wg.Wait()

	var packages []pkg
	for _, r := range results {
		packages = append(packages, r...)
	}
	return packages, failed
}

// printPackages prints packages as aligned "product  name  version-release"
// columns.
func printPackages(w io.Writer, packages []pkg) {
	var productWidth, nameWidth int
	for _, p := range packages {
		productWidth = max(productWidth, len(p.product))
		nameWidth = max(nameWidth, len(p.name))
	}
	for _, p := range packages {
		fmt.Fprintf(w, "%-*s  %-*s  %s\n", productWidth, p.product, nameWidth, p.name, p.version)
	}
}

// logError prints err to w as "ERROR    message".
func logError(w io.Writer, err error) {
	fmt.Fprintf(w, "%-8s %v\n", "ERROR", err)
}
