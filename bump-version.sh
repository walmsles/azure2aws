#!/bin/bash
set -e

CURRENT=$(cat version.txt)
echo "Current version: $CURRENT"

IFS='.' read -r -a parts <<< "$CURRENT"
MAJOR=${parts[0]}
MINOR=${parts[1]}
PATCH=${parts[2]}

case "${1:-patch}" in
  major)
    MAJOR=$((MAJOR + 1))
    MINOR=0
    PATCH=0
    ;;
  minor)
    MINOR=$((MINOR + 1))
    PATCH=0
    ;;
  patch)
    PATCH=$((PATCH + 1))
    ;;
  *)
    echo "Usage: $0 [major|minor|patch]"
    exit 1
    ;;
esac

NEW_VERSION="$MAJOR.$MINOR.$PATCH"
echo "$NEW_VERSION" > version.txt
echo "Bumped to: $NEW_VERSION"
