#!/bin/sh
# Writes the Homebrew formula and the Scoop manifest for one release. Both
# say where the files of the release are and what their checksums must be.
#
#   packages.sh v0.2.0 <folder with the built files> <folder of the tap repository>
set -eu
tag=$1 dist=$2 tap=$3

base=https://github.com/vedantlavale/taasant/releases/download/$tag
about="Encrypted file storage in a Telegram chat, used from the terminal"
sum() { shasum -a 256 "$dist/$1" | cut -d' ' -f1; }

mkdir -p "$tap/Formula" "$tap/bucket"

cat > "$tap/Formula/taasant.rb" <<END
class Taasant < Formula
  desc "$about"
  homepage "https://taasant.vedantworks.com"
  version "${tag#v}"

  on_macos do
    on_arm do
      url "$base/taasant-macos-arm64"
      sha256 "$(sum taasant-macos-arm64)"
    end
    on_intel do
      url "$base/taasant-macos-intel"
      sha256 "$(sum taasant-macos-intel)"
    end
  end
  on_linux do
    on_intel do
      url "$base/taasant-linux-amd64"
      sha256 "$(sum taasant-linux-amd64)"
    end
  end

  def install
    bin.install Dir["taasant-*"].first => "taasant"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/taasant version")
  end
end
END

cat > "$tap/bucket/taasant.json" <<END
{
  "version": "${tag#v}",
  "description": "$about",
  "homepage": "https://taasant.vedantworks.com",
  "architecture": {
    "64bit": {
      "url": "$base/taasant-windows.exe#/taasant.exe",
      "hash": "$(sum taasant-windows.exe)"
    }
  },
  "bin": "taasant.exe"
}
END
