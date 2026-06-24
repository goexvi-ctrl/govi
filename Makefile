# Build govi binaries with git-derived version metadata.
VERSION_LDFLAGS := $(shell ./scripts/version-ldflags.sh)

all: build

.PHONY: build nvi test

build: nvi gui/build/Govi.app

nvi:
	go build -ldflags "$(VERSION_LDFLAGS)" -o nvi ./cmd/nvi

include gui/govi.mk

gui/build/Govi.app: govi-app

test:
	go test ./...

IDIR=$(HOME)/bin
install: $(IDIR)/gnvi $(IDIR)/Govi.app $(IDIR)/govi

$(IDIR)/gnvi: nvi
	cp $< $@

$(IDIR)/Govi.app: gui/build/Govi.app
	cp -r $< $@

$(IDIR)/govi: gui/govi
	cp $< $@

clean:
	rm -rf gui/build/Govi.app nvi
