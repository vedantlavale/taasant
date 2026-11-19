#!/bin/sh
# Builds dist/Taasant.app and dist/Taasant.dmg, the installer people download. The app is a window and a menu
# bar icon around the taasant program, which is put inside it.
#
#   macos/build.sh            both kinds of Mac
#   macos/build.sh arm64      only this Mac, quicker
set -e
cd "$(dirname "$0")"
root=$(cd .. && pwd)
dist=$PWD/dist
app=$dist/Taasant.app
version=$(git -C "$root" describe --tags --abbrev=0 2>/dev/null | sed 's/^v//')
version=${version:-0.0.0}

rm -rf "$dist"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources" "$dist/go"

if [ "$1" = arm64 ]; then archs="arm64"; swift_archs="--arch arm64"; else archs="arm64 amd64"; swift_archs="--arch arm64 --arch x86_64"; fi

for arch in $archs; do
  (cd "$root" && CGO_ENABLED=0 GOOS=darwin GOARCH=$arch go build -trimpath -o "$dist/go/taasant-$arch" ./cmd/taasant)
done
lipo -create "$dist"/go/taasant-* -output "$app/Contents/Resources/taasant"

swift build -c release $swift_archs
cp "$(swift build -c release $swift_archs --show-bin-path)/Taasant" "$app/Contents/MacOS/Taasant"
cp Sources/Taasant/Resources/ant*.png "$app/Contents/Resources/"

sed "s/VERSION/$version/" Info.plist > "$app/Contents/Info.plist"
cp AppIcon.icns "$app/Contents/Resources/AppIcon.icns"
rm -rf "$dist/go"

# Without a certificate. Ad hoc signing is still needed: a program on an
# Apple silicon Mac does not start with no signature at all.
codesign --force --deep --sign - "$app"
# The installer: a window with the app, a shortcut to Applications and an
# arrow between them. Finder lays the window out from a script, which needs
# a login session; where there is none, the disk image is plain but still works.
name=Taasant
# Not "Taasant": the drive in Finder is /Volumes/taasant, and the two would clash.
volume="Install taasant"
stage=$dist/dmg
rm -rf "$stage" "$dist/$name.dmg" "$dist/rw.dmg"
mkdir -p "$stage/.background"
cp -R "$app" "$stage/"
ln -s /Applications "$stage/Applications"
cp dmg/background.tiff "$stage/.background/background.tiff"
cp AppIcon.icns "$stage/.VolumeIcon.icns"
SetFile -a C "$stage" 2>/dev/null || true

# A copy left mounted by an earlier run would take the name, and Finder would lay out the wrong one.
for old in /Volumes/"$volume"*; do hdiutil detach -quiet -force "$old" 2>/dev/null || true; done
hdiutil create -quiet -srcfolder "$stage" -volname "$volume" -fs HFS+ -format UDRW -ov "$dist/rw.dmg"
device=$(hdiutil attach -readwrite -noverify -noautoopen "$dist/rw.dmg" | awk '/\/Volumes\//{print $1; exit}')
if osascript >/dev/null 2>&1 <<LAYOUT
tell application "Finder"
  tell disk "$volume"
    open
    set current view of container window to icon view
    set toolbar visible of container window to false
    set statusbar visible of container window to false
    set the bounds of container window to {200, 120, 860, 520}
    set opts to the icon view options of container window
    set arrangement of opts to not arranged
    set icon size of opts to 112
    set text size of opts to 13
    set background picture of opts to file ".background:background.tiff"
    set position of item "$name.app" of container window to {180, 215}
    set position of item "Applications" of container window to {480, 215}
    repeat with leftover in {".background", ".fseventsd", ".VolumeIcon.icns", ".Trashes", ".DS_Store"}
      try
        set position of item leftover of container window to {1200, 1200}
      end try
    end repeat
    close
    open
    update without registering applications
    delay 2
    close
  end tell
end tell
LAYOUT
then :; else echo "warning: Finder could not lay out the installer window, so it will be plain" >&2; fi
sync
hdiutil detach -quiet "$device" || { sleep 2; hdiutil detach -force -quiet "$device"; }
hdiutil convert -quiet "$dist/rw.dmg" -format UDZO -imagekey zlib-level=9 -o "$dist/$name.dmg"
rm -rf "$dist/rw.dmg" "$stage"
echo "built $app ($version) and $dist/$name.dmg"
