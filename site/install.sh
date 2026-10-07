#!/bin/sh
# Installs the newest taasant on macOS or Linux:
#
#   curl -fsSL https://taasant.vedantworks.com/install.sh | sh
#
# It goes into /usr/local/bin. Set TAASANT_DIR to put it somewhere else.
set -eu

case "$(uname -s) $(uname -m)" in
  "Darwin arm64")  file=taasant-macos-arm64 ;;
  "Darwin x86_64") file=taasant-macos-intel ;;
  "Linux x86_64")  file=taasant-linux-amd64 ;;
  *)
    echo "There is no download for this kind of computer ($(uname -s) $(uname -m))." >&2
    echo "With Go installed, use: go install github.com/vedantlavale/taasant/cmd/taasant@latest" >&2
    exit 1 ;;
esac

dir=${TAASANT_DIR:-/usr/local/bin}
new=$(mktemp)
trap 'rm -f "$new"' EXIT

echo "Downloading $file"
curl -fL --progress-bar -o "$new" "https://github.com/vedantlavale/taasant/releases/latest/download/$file"
chmod 755 "$new"

# /usr/local/bin belongs to the administrator on most computers.
if [ -d "$dir" ] && [ -w "$dir" ]; then
  mv "$new" "$dir/taasant"
else
  echo "Putting taasant into $dir needs your password."
  sudo mkdir -p "$dir"
  sudo mv "$new" "$dir/taasant"
fi
echo "taasant is installed in $dir. Start it with: taasant"
