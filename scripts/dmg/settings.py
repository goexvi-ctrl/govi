# dmgbuild settings for the GoVi release disk image.
#
# Used by scripts/macos-release.sh:
#   APP=... CLI=... BG=... dmgbuild -s scripts/dmg/settings.py "<VolName>" out.dmg
#
# It lays out a two-row "drag to install" window over a background that has an
# arrow above each row (scripts/dmg/dmg-background.png): GoVi.app -> Applications
# on top, and the govi CLI -> /usr/local/bin below. dmgbuild writes the .DS_Store
# directly, so no Finder/GUI session is needed (works in CI).
#
# Icon coordinates are window-content points (top-left origin) chosen to sit
# under the arrow endpoints; keep them in sync with scripts/dmg/make-background.sh
# if the background geometry changes.

import os

app = os.environ["APP"]   # path to the built (signed) GoVi.app
cli = os.environ["CLI"]   # path to the built (signed) govi CLI
bg = os.environ["BG"]     # path to the background .tiff (1x + 2x reps)

appname = os.path.basename(app)
cliname = os.path.basename(cli)

# Contents of the image.
files = [app, cli]
symlinks = {"Applications": "/Applications", "usr-local-bin": "/usr/local/bin"}

# Window and icon appearance.
background = bg
default_view = "icon-view"
window_rect = ((200, 150), (640, 500))  # ((x, y), (width, height))
icon_size = 96
text_size = 12
show_status_bar = False
show_toolbar = False
show_pathbar = False
show_sidebar = False

# Icon positions (centers), under each arrow endpoint in the background.
icon_locations = {
    appname:         (245, 212),
    "Applications":  (395, 212),
    cliname:         (245, 391),
    "usr-local-bin": (395, 391),
}
