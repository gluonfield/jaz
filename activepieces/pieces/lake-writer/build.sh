#!/usr/bin/env bash
set -euo pipefail
# Builds the lake-writer piece into ./dist/*.tgz (where the compose
# piece-installer looks). The Activepieces monorepo is only a build env.
# Usage: ./build.sh   (set AP_DIR to reuse an existing activepieces checkout)

AP_DIR="${AP_DIR:-$HOME/Projects/vendor/activepieces}"
PIECE_SRC="$(cd "$(dirname "$0")" && pwd)"
DEST="packages/pieces/custom/lake-writer"

if [ ! -d "$AP_DIR" ]; then
  git clone --depth 1 https://github.com/activepieces/activepieces "$AP_DIR"
fi
cd "$AP_DIR"
[ -d node_modules ] || npm ci

if [ ! -d "$DEST" ]; then
  echo "Scaffolding piece (answer prompts: name=lake-writer, type=custom)..."
  npm run cli pieces create
fi

rsync -a --delete "$PIECE_SRC/src/" "$DEST/src/"
cp "$PIECE_SRC/package.json" "$DEST/package.json"

npm run build-piece lake-writer

mkdir -p "$PIECE_SRC/dist"
rm -f "$PIECE_SRC/dist/"*.tgz
cp "$DEST"/dist/*.tgz "$PIECE_SRC/dist/"
echo "Built into $PIECE_SRC/dist/:"
ls "$PIECE_SRC/dist/"*.tgz
echo "Now: cd ../../.. && docker compose up -d   (installer picks it up)"
