#!/bin/sh
# Prints the version that comes after the last one, going by the labels of
# the pull request that was merged. Prints nothing if no label asks for a
# release.
#
#   next-version.sh v0.2.0 "documentation release:minor"   prints v0.3.0
#
# Going from 1.x to 2.0.0 needs more than a label: Go only accepts v2 tags
# once the module path in go.mod ends in /v2. Until then such a release
# stops at the version check and publishes nothing.
set -eu
last=${1:-v0.0.0} labels=${2:-}

# The version is three numbers: major.minor.patch.
set -- $(echo "${last#v}" | tr . ' ')
major=$1 minor=$2 patch=$3

case " $(echo "$labels" | tr '\n' ' ') " in
  *" release:major "*) major=$((major + 1)) minor=0 patch=0 ;;
  *" release:minor "*) minor=$((minor + 1)) patch=0 ;;
  *" release:patch "*) patch=$((patch + 1)) ;;
  *) exit 0 ;;
esac
echo "v$major.$minor.$patch"
