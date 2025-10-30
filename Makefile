.PHONY: default
default: build

GOOS?=linux
GOARCH?=amd64
ROOT_PKG=github.com/pbabilas/pinpoint

.PHONY: build
build:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build \
	  -installsuffix cgo \
	  -o bin/pinpoint-$(GOOS)-$(GOARCH) \
	  -ldflags "-X main.Version=$(VERSION)" \
	  $(ROOT_PKG)