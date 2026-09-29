FROM	registry.opensuse.org/opensuse/bci/golang:latest AS build

WORKDIR	/src
COPY	go.mod go.sum ./
RUN	go mod download
COPY	*.go ./
ARG	VERSION=dev
RUN	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /susepkg .

FROM	registry.opensuse.org/opensuse/bci/bci-micro:latest AS certs
FROM	scratch

COPY	--from=certs /etc/ssl/ca-bundle.pem /etc/ssl/certs/ca-certificates.crt
COPY	--from=build /susepkg /susepkg

USER	65534:65534
ENTRYPOINT ["/susepkg"]
