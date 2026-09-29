BIN      := susepkg
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed "s/^v//" | grep . || echo dev)
GO       ?= go
LDFLAGS  := -s -w -X main.version=$(VERSION)

PREFIX   ?= $(HOME)
BINDIR   ?= $(PREFIX)/bin

SRC      := $(wildcard *.go) go.mod go.sum

.DEFAULT_GOAL := build

.PHONY: build
build: $(BIN)

$(BIN): $(SRC)
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $@ .

.PHONY: test
test:
	$(GO) test -race -cover ./...

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: fmt
fmt:
	golangci-lint fmt ./...

.PHONY: fmt-check
fmt-check:
	golangci-lint fmt --diff ./...

.PHONY: vet
vet:
	$(GO) vet ./...

.PHONY: tidy-check
tidy-check:
	$(GO) mod tidy -diff

.PHONY: check
check: fmt-check vet lint test tidy-check

.PHONY: install
install: $(BIN)
	install -d $(DESTDIR)$(BINDIR)
	install -m 0755 $(BIN) $(DESTDIR)$(BINDIR)/$(BIN)

.PHONY: uninstall
uninstall:
	rm -f $(DESTDIR)$(BINDIR)/$(BIN)

.PHONY: clean
clean:
	rm -f $(BIN)
