.PHONY: default
default: build

GOOS?=linux
GOARCH?=amd64
ROOT_PKG=b-code.cloud/routeros/ovpn

.PHONY: build
build:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build \
	  -installsuffix cgo \
	  -o bin/routeros-util.$(GOOS)_$(GOARCH) \
	  -ldflags "-X main.Version=$(VERSION)" \
	  $(ROOT_PKG)