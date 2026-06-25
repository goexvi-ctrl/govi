# Build govi binaries with git-derived version metadata.
VERSION_LDFLAGS := $(shell ./scripts/version-ldflags.sh)

all: build

.PHONY: build govi test

build: govi gui/build/GoVi.app

govi:
	go build -ldflags "$(VERSION_LDFLAGS)" -o govi ./cmd/govi

include gui/govi.mk

gui/build/GoVi.app: govi-app

test:
	go test ./...

IDIR=$(HOME)/bin
install: $(IDIR)/govi $(IDIR)/GoVi.app

$(IDIR)/govi: govi
	cp $< $@

# ditto after rm -rf: cp -r into an existing .app nest a second bundle
# (~/bin/GoVi.app/GoVi.app) and leave the outer app without Resources/.
$(IDIR)/GoVi.app: gui/build/GoVi.app $(GOVI_ICNS)
	rm -rf $@
	ditto gui/build/GoVi.app $@

clean:
	rm -rf govi gui/build cmd/govi/govi
