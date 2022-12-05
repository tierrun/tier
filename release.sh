#!/bin/sh
set -eu

IFS=".$IFS" read -r major minor patch <VERSION.txt
git_hash=$(git rev-parse HEAD)
if ! git diff-index --quiet HEAD; then
	git_hash="${git_hash}-dirty"
fi
base_hash=$(git rev-list --max-count=1 HEAD -- VERSION.txt)
change_count=$(git rev-list --count HEAD "^$base_hash")
short_hash=$(echo "$git_hash" | cut -c1-9)

long_suffix="-t$short_hash"
MINOR="$major.$minor"
SHORT="$MINOR.$patch"
LONG="${SHORT}$long_suffix"
GIT_HASH="$git_hash"

if [ "$1" = "shellvars" ]; then
	cat <<EOF
VERSION_MINOR="$MINOR"
VERSION_SHORT="$SHORT"
VERSION_LONG="$LONG"
VERSION_GIT_HASH="$GIT_HASH"
EOF
	exit 0
fi

export VERSION_MINOR="$MINOR"
export VERSION_SHORT="$SHORT"
export VERSION_LONG="$LONG"
export VERSION_GIT_HASH="$GIT_HASH"

goreleaser release --rm-dist --skip-publish "$@"
