#!/bin/sh
# Refresh the desktop caches after the files are gone, so a removed app stops
# appearing in the menu and .age files lose its icon.
#
# Every command is best-effort: these tools are absent on minimal systems, and a
# missing menu entry must never fail the install of a working binary.
set -e

if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database -q /usr/share/applications || true
fi

if command -v update-mime-database >/dev/null 2>&1; then
    update-mime-database /usr/share/mime || true
fi

if command -v gtk-update-icon-cache >/dev/null 2>&1; then
    gtk-update-icon-cache -q -t -f /usr/share/icons/hicolor || true
fi

exit 0
