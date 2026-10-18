# dmgbuild settings for Highlighter's .dmg (see scripts/package.sh).
#
# The window looks like most Mac app installers: a fixed 660x400 window with
# no toolbar or sidebar, the app on the left, an arrow on the background and
# Applications on the right. dmgbuild writes Finder's layout file directly,
# so this works on CI without scripting Finder.
import os.path

app = defines["app"]  # noqa: F821 - provided by dmgbuild
appname = os.path.basename(app)

format = "ULMO"  # LZMA: smaller than the zlib (UDZO) default
filesystem = "HFS+"
files = [app]
symlinks = {"Applications": "/Applications"}
icon = defines["volume_icon"]  # noqa: F821

background = defines["background"]  # noqa: F821
window_rect = ((200, 120), (660, 400))
default_view = "icon-view"
show_status_bar = False
show_tab_view = False
show_toolbar = False
show_pathbar = False
show_sidebar = False
show_icon_preview = False
include_icon_view_settings = True
arrange_by = None
icon_size = 128
text_size = 13
label_pos = "bottom"
icon_locations = {appname: (170, 190), "Applications": (490, 190)}
