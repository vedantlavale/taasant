#!/bin/sh
# Builds dist/Taasant.app and dist/Taasant.zip. The app is a window and a menu
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
ditto -c -k --keepParent "$app" "$dist/Taasant.zip"
echo "built $app ($version)"
