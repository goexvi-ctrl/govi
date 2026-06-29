# Build govi binaries with git-derived version metadata.
VERSION_LDFLAGS := $(shell ./scripts/version-ldflags.sh)

all: build

COVER_PROFILE ?= cover.out
COVER_HTML ?= cover.html
# gui/bridge is cgo (//export); its profile breaks go tool cover.
COVER_PKGS := $(shell go list ./... | grep -v '/gui/bridge$$')

.PHONY: build govi test coverage coverage-report coverage-html release

build: govi gui/build/GoVi.app

govi:
	go build -ldflags "$(VERSION_LDFLAGS)" -o govi ./cmd/govi

include gui/govi.mk

gui/build/GoVi.app: govi-app

test:
	go test ./...

# coverage runs all tests and writes $(COVER_PROFILE). coverage-report prints
# per-function totals; coverage-html writes an interactive report.
coverage: $(COVER_PROFILE)
	@go tool cover -func=$(COVER_PROFILE) | tail -1

$(COVER_PROFILE):
	go test $(COVER_PKGS) -coverprofile=$(COVER_PROFILE) -covermode=atomic

coverage-report: $(COVER_PROFILE)
	go tool cover -func=$(COVER_PROFILE)

coverage-html: $(COVER_PROFILE)
	go tool cover -html=$(COVER_PROFILE) -o $(COVER_HTML)
	./scripts/cover-html-light.sh $(COVER_HTML)
	@echo "Wrote $(COVER_HTML)"

IDIR=$(HOME)/bin
install: $(IDIR)/govi $(IDIR)/GoVi.app

$(IDIR)/govi: govi
	cp $< $@

# ditto after rm -rf: cp -r into an existing .app nest a second bundle
# (~/bin/GoVi.app/GoVi.app) and leave the outer app without Resources/.
LSREGISTER = /System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister
$(IDIR)/GoVi.app: gui/build/GoVi.app $(GOVI_ICNS)
	rm -rf $@
	ditto gui/build/GoVi.app $@
	$(LSREGISTER) -f $@   # register file types + the govi:// URL scheme

# release: zip the macOS app bundle for upload to a GitHub release. Use ditto
# (not zip) so the bundle's symlinks and extended attributes survive. The
# version is taken from the latest git tag; arch from the build host.
RELEASE_VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//')
RELEASE_ARCH    := $(shell uname -m)
RELEASE_ZIP     := gui/build/GoVi-$(RELEASE_VERSION)-macos-$(RELEASE_ARCH).zip

release: gui/build/GoVi.app
	rm -f $(RELEASE_ZIP)
	ditto -c -k --keepParent gui/build/GoVi.app $(RELEASE_ZIP)
	@echo "Wrote $(RELEASE_ZIP)"

clean:
	rm -rf govi gui/build cmd/govi/govi
	rm -f $(COVER_PROFILE) $(COVER_HTML)
