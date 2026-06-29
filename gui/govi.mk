# Dependency-aware build rules for the macOS GoVi.app bundle.
#
# Included from the repository Makefile:
#   include gui/govi.mk
#
# Or built directly:
#   make -f gui/govi.mk
#   make -f gui/govi.mk -C /path/to/govi   # from anywhere

GOVI_GUI_DIR := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
GOVI_ROOT    := $(GOVI_GUI_DIR)/..

GOVI_BUILD   := $(GOVI_GUI_DIR)/build
GOVI_LIB     := $(GOVI_BUILD)/libgovi.a
GOVI_LIB_HDR := $(GOVI_BUILD)/libgovi.h
GOVI_BIN     := $(GOVI_BUILD)/GoVi
GOVI_APP     := $(GOVI_BUILD)/GoVi.app
GOVI_PLIST   := $(GOVI_APP)/Contents/Info.plist
GOVI_EXE     := $(GOVI_APP)/Contents/MacOS/GoVi

# Architectures to build. Default to the build host; set "arm64 x86_64" for a
# universal (fat) binary:
#   make GOVI_ARCHS="arm64 x86_64"
GOVI_ARCHS     ?= $(shell uname -m)
# Minimum macOS the release targets. 11 (Big Sur) is Go 1.26's supported floor
# and the lowest the Swift sources compile against cleanly; it reaches Intel
# Macs back to ~2013. Applied to both the Swift slices and the Go (cgo) builds.
GOVI_MACOS_MIN ?= 11

GOVI_SWIFT_SRC := $(wildcard $(GOVI_GUI_DIR)/macos/*.swift)
GOVI_BRIDGE_GO := $(wildcard $(GOVI_GUI_DIR)/bridge/*.go)
GOVI_PLIST_SRC := $(GOVI_GUI_DIR)/macos/Info.plist
GOVI_ICON_SRC  := $(GOVI_ROOT)/icon.png
GOVI_ICON_SH   := $(GOVI_GUI_DIR)/macos/make-icon.sh
GOVI_ICNS      := $(GOVI_APP)/Contents/Resources/AppIcon.icns

# Every non-standard Go source file reachable from gui/bridge (engine, grid, …).
GOVI_GO_SRC := $(shell cd $(GOVI_ROOT) && \
	go list -deps -f '{{if not .Standard}}{{range .GoFiles}}{{$$.Dir}}/{{.}} {{end}}{{end}}' \
	./gui/bridge 2>/dev/null)

GOVI_MOD     := $(GOVI_ROOT)/go.mod $(GOVI_ROOT)/go.sum
GOVI_VERSION := $(GOVI_ROOT)/scripts/version-ldflags.sh
GOVI_GIT     := $(wildcard $(GOVI_ROOT)/.git/HEAD $(GOVI_ROOT)/.git/index)

# Written before each libgovi build so a dirty tree gets a fresh build timestamp.
GOVI_VERSION_FLAGS := $(GOVI_BUILD)/version.flags

# Rebuild libgovi on every make when the tree is dirty so :version's build
# timestamp reflects this build, not an earlier cached artifact. Forces a full
# cgo re-archive each iteration while developing with local edits.
GOVI_TREE_DIRTY := $(shell cd $(GOVI_ROOT) && rc=0; \
	git rev-parse -q --verify HEAD >/dev/null 2>&1 && { git diff-index --quiet HEAD -- 2>/dev/null || rc=$$?; }; \
	test $$rc -eq 1 && echo 1)
ifneq ($(GOVI_TREE_DIRTY),)
.PHONY: $(GOVI_BIN)
endif

.DEFAULT_GOAL := govi-app

.PHONY: govi-app govi-clean

govi-app: $(GOVI_APP)

# Build the GoVi executable: one Mach-O slice per arch in GOVI_ARCHS, lipo'd
# into a single binary (universal when GOVI_ARCHS lists more than one). Each
# slice links the cgo c-archive built for the matching Go arch. The generated
# libgovi.h is identical for every 64-bit arch, so one copy serves the Bridging
# header for all slices.
$(GOVI_BIN): $(GOVI_VERSION) $(GOVI_GO_SRC) $(GOVI_BRIDGE_GO) $(GOVI_MOD) $(GOVI_GIT) $(GOVI_SWIFT_SRC) $(GOVI_GUI_DIR)/macos/Bridging.h
	@mkdir -p $(GOVI_BUILD)
	@$(GOVI_VERSION) > $(GOVI_VERSION_FLAGS)
	@set -e; slices=""; \
	for arch in $(GOVI_ARCHS); do \
	  case $$arch in \
	    arm64)  goarch=arm64 ;; \
	    x86_64) goarch=amd64 ;; \
	    *) echo "govi.mk: unsupported GOVI_ARCHS entry '$$arch'" >&2; exit 1 ;; \
	  esac; \
	  lib=$(GOVI_BUILD)/libgovi-$$arch.a; \
	  slice=$(GOVI_BUILD)/GoVi-$$arch; \
	  echo ">> libgovi ($$arch)"; \
	  ( cd $(GOVI_ROOT) && MACOSX_DEPLOYMENT_TARGET=$(GOVI_MACOS_MIN) \
	    CGO_ENABLED=1 GOARCH=$$goarch CC="clang -arch $$arch -mmacosx-version-min=$(GOVI_MACOS_MIN)" \
	    go build -ldflags "$$(cat $(GOVI_VERSION_FLAGS))" \
	    -buildmode=c-archive -o $$lib ./gui/bridge ); \
	  cp $(GOVI_BUILD)/libgovi-$$arch.h $(GOVI_LIB_HDR); \
	  echo ">> GoVi ($$arch)"; \
	  swiftc -O -target $$arch-apple-macosx$(GOVI_MACOS_MIN) \
	    -import-objc-header $(GOVI_GUI_DIR)/macos/Bridging.h \
	    -I $(GOVI_BUILD) \
	    $(GOVI_SWIFT_SRC) $$lib \
	    -framework Cocoa -framework CoreFoundation -framework Security \
	    -o $$slice; \
	  slices="$$slices $$slice"; \
	done; \
	lipo -create $$slices -output $(GOVI_BIN)

$(GOVI_EXE): $(GOVI_BIN) | $(GOVI_APP)/Contents/MacOS
	cp $(GOVI_BIN) $(GOVI_EXE)

$(GOVI_APP)/Contents/MacOS:
	@mkdir -p $@

$(GOVI_PLIST): $(GOVI_PLIST_SRC) | $(GOVI_APP)/Contents
	@mkdir -p $(dir $@)
	cp $(GOVI_PLIST_SRC) $@

$(GOVI_APP)/Contents:
	@mkdir -p $@

$(GOVI_ICNS): $(GOVI_ICON_SRC) $(GOVI_ICON_SH) | $(GOVI_APP)/Contents/Resources
	$(GOVI_ICON_SH) $(GOVI_ICON_SRC) $(GOVI_ICNS)

$(GOVI_APP)/Contents/Resources:
	@mkdir -p $@

# Ad-hoc sign the assembled bundle. swiftc/ld only linker-sign the executable,
# leaving Info.plist and Resources unbound -- which reads as an invalid (not
# merely unsigned) signature once a download is quarantined, so Gatekeeper
# reports the app as "damaged" on Apple Silicon. A full ad-hoc signature
# (-s -) binds the whole bundle; the app is still unsigned by a Developer ID,
# so a quarantined copy needs its quarantine cleared, but it is no longer
# flagged as damaged. Re-run on every build of the bundle (it is cheap).
.PHONY: $(GOVI_APP)
$(GOVI_APP): $(GOVI_EXE) $(GOVI_PLIST) $(GOVI_ICNS)
	codesign --force --sign - --identifier org.govi.editor $(GOVI_APP)

govi-clean:
	rm -rf $(GOVI_BUILD)