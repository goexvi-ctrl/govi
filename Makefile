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

# release: build a signed macOS .dmg for upload to a GitHub release. The image
# holds the GoVi.app bundle, the govi CLI, and an /Applications symlink for
# drag-install. Version comes from the latest git tag; arch from the build host.
#
# Default builds are ad-hoc signed: runnable locally, but a download is
# quarantined and must be de-quarantined by hand. Pass a Developer ID to get a
# hardened-runtime, notarized, stapled image that opens with no user fiddling:
#
#   make release \
#     CODESIGN_IDENTITY="Developer ID Application: Name (TEAMID)" \
#     NOTARY_PROFILE=govi-notary
#
# NOTARY_PROFILE names a stored `xcrun notarytool store-credentials` profile.
RELEASE_VERSION   ?= $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//')
RELEASE_ARCH      := $(shell uname -m)
RELEASE_DMG       := gui/build/GoVi-$(RELEASE_VERSION)-macos-$(RELEASE_ARCH).dmg
CODESIGN_IDENTITY ?= -
NOTARY_PROFILE    ?= govi-notary

release: govi gui/build/GoVi.app
	APP=gui/build/GoVi.app CLI=govi DMG=$(RELEASE_DMG) \
	VOLNAME="GoVi $(RELEASE_VERSION)" IDENTITY="$(CODESIGN_IDENTITY)" \
	NOTARY_PROFILE="$(NOTARY_PROFILE)" ./scripts/macos-release.sh

clean:
	rm -rf govi gui/build cmd/govi/govi
	rm -f $(COVER_PROFILE) $(COVER_HTML)
