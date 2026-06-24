# Dependency-aware build rules for the macOS Govi.app bundle.
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
GOVI_BIN     := $(GOVI_BUILD)/Govi
GOVI_APP     := $(GOVI_BUILD)/Govi.app
GOVI_PLIST   := $(GOVI_APP)/Contents/Info.plist
GOVI_EXE     := $(GOVI_APP)/Contents/MacOS/Govi

GOVI_SWIFT_SRC := $(wildcard $(GOVI_GUI_DIR)/macos/*.swift)
GOVI_BRIDGE_GO := $(wildcard $(GOVI_GUI_DIR)/bridge/*.go)
GOVI_PLIST_SRC := $(GOVI_GUI_DIR)/macos/Info.plist
GOVI_ICON_SRC  := $(GOVI_ROOT)/icon.jpg
GOVI_ICON_SH   := $(GOVI_GUI_DIR)/macos/make-icon.sh
GOVI_ICNS      := $(GOVI_APP)/Contents/Resources/AppIcon.icns

# Every non-standard Go source file reachable from gui/bridge (engine, grid, …).
GOVI_GO_SRC := $(shell cd $(GOVI_ROOT) && \
	go list -deps -f '{{if not .Standard}}{{range .GoFiles}}{{$$.Dir}}/{{.}} {{end}}{{end}}' \
	./gui/bridge 2>/dev/null)

GOVI_MOD     := $(GOVI_ROOT)/go.mod $(GOVI_ROOT)/go.sum
GOVI_VERSION := $(GOVI_ROOT)/scripts/version-ldflags.sh
GOVI_GIT     := $(wildcard $(GOVI_ROOT)/.git/HEAD $(GOVI_ROOT)/.git/index)

# Recomputed when HEAD/index or sources change so :version metadata stays current.
GOVI_VERSION_FLAGS := $(GOVI_BUILD)/version.flags

.DEFAULT_GOAL := govi-app

.PHONY: govi-app govi-clean

govi-app: $(GOVI_APP)

$(GOVI_VERSION_FLAGS): $(GOVI_VERSION) $(GOVI_GO_SRC) $(GOVI_BRIDGE_GO) $(GOVI_MOD) $(GOVI_GIT)
	@mkdir -p $(GOVI_BUILD)
	$(GOVI_VERSION) > $@

$(GOVI_LIB) $(GOVI_LIB_HDR): $(GOVI_GO_SRC) $(GOVI_BRIDGE_GO) $(GOVI_MOD) $(GOVI_VERSION_FLAGS)
	@mkdir -p $(GOVI_BUILD)
	cd $(GOVI_ROOT) && go build -ldflags "$$(cat $(GOVI_VERSION_FLAGS))" \
		-buildmode=c-archive -o $(GOVI_LIB) ./gui/bridge

$(GOVI_BIN): $(GOVI_LIB) $(GOVI_LIB_HDR) $(GOVI_SWIFT_SRC) $(GOVI_GUI_DIR)/macos/Bridging.h
	@mkdir -p $(GOVI_BUILD)
	swiftc -O \
		-import-objc-header $(GOVI_GUI_DIR)/macos/Bridging.h \
		-I $(GOVI_BUILD) \
		$(GOVI_SWIFT_SRC) \
		$(GOVI_LIB) \
		-framework Cocoa \
		-framework CoreFoundation \
		-framework Security \
		-o $(GOVI_BIN)

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

$(GOVI_APP): $(GOVI_EXE) $(GOVI_PLIST) $(GOVI_ICNS)

govi-clean:
	rm -rf $(GOVI_BUILD)