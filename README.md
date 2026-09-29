![Build Status](https://github.com/ricardobranco777/go-susepkg/actions/workflows/ci.yml/badge.svg)

# susepkg

Show SUSE package versions using [documented API](https://scc.suse.com/api/package_search/v4/documentation)

Go rewrite of [susepkg](https://github.com/ricardobranco777/susepkg): same options and output, as a single static binary with no Python or librpm.

Docker image available at `ghcr.io/ricardobranco777/go-susepkg:latest`

## Build

Requires Go 1.27+. Development checks (`make check`) also need [golangci-lint](https://golangci-lint.run) v2.

```
make build
make install   # installs to ~/bin
```

## Usage

```
usage: susepkg [-h] [-a ARCH] [-i] -p PRODUCT [-x] [--version] [package]

show SUSE package versions

positional arguments:
  package  may be a shell pattern or regular expression

options:
  -a, --arch string           architecture: aarch64, ppc64le, s390x, x86_64
  -h, --help                  show this help message and exit
  -i, --insensitive           case insensitive search
  -p, --product stringArray   product or 'list' or 'any'. May be specified multiple times
  -x, --regex                 search regular expression
      --version               show program's version number and exit
```

Set `DEBUG=1` to dump HTTP requests and responses to stderr.

## Example usage

- `susepkg -p any podman`
- `susepkg -p weed podman`
- `susepkg -p Micro podman`
- `susepkg -p SLES podman`
- `susepkg -p SL-Micro/6.0 \*podman\*`
- `susepkg -p SL-Micro/6.0 -x podman-.*`

## Differences from the Python version

- Regular expressions use Go's RE2 syntax: no backreferences or lookarounds.
- Malformed product names such as `Micro/5` are passed through unchanged instead of raising an exception.

## Product list

Run `susepkg -p list`.
