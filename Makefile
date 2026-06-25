# Build govi binaries with git-derived version metadata.
VERSION_LDFLAGS := $(shell ./scripts/version-ldflags.sh)

all: build

.PHONY: build govi test

build: govi gui/build/Govi.app

govi:
	go build -ldflags "$(VERSION_LDFLAGS)" -o govi ./cmd/govi

include gui/govi.mk

gui/build/Govi.app: govi-app

test:
	go test ./...

IDIR=$(HOME)/bin
install: $(IDIR)/govi $(IDIR)/Govi.app

$(IDIR)/govi: govi
	cp $< $@

# ditto after rm -rf: cp -r into an existing .app nest a second bundle
# (~/bin/Govi.app/Govi.app) and leave the outer app without Resources/.
$(IDIR)/Govi.app: gui/build/Govi.app $(GOVI_ICNS)
	rm -rf $@
	ditto gui/build/Govi.app $@

clean:
	rm -rf govi gui/build cmd/govi/govi
