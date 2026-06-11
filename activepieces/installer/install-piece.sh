#!/bin/sh
# One-shot piece installer: runs inside the compose network on every `up`,
# exits 0 quickly when the piece version is already installed.
set -eu

apk add --no-cache curl >/dev/null 2>&1

PIECE_NAME=$(sed -n 's/.*"name": *"\([^"]*\)".*/\1/p' /piece-package.json)
PIECE_VERSION=$(sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' /piece-package.json)
TGZ=$(ls /piece-dist/*.tgz 2>/dev/null | head -1 || true)

if [ -z "$TGZ" ]; then
  echo "piece-installer: no .tgz in pieces/lake-writer/dist/ — run pieces/lake-writer/build.sh first. Skipping."
  exit 0
fi

echo "piece-installer: waiting for activepieces..."
i=0
until curl -sf "$AP_URL/api/v1/flags" >/dev/null 2>&1; do
  i=$((i + 1))
  [ "$i" -gt 120 ] && echo "piece-installer: timed out waiting for AP" && exit 1
  sleep 2
done

sign_in() {
  curl -sf "$AP_URL/api/v1/authentication/sign-in" \
    -H 'Content-Type: application/json' \
    -d "{\"email\":\"$AP_EMAIL\",\"password\":\"$AP_PASSWORD\"}" 2>/dev/null || true
}

RESP=$(sign_in)
TOKEN=$(printf '%s' "$RESP" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
if [ -z "$TOKEN" ]; then
  echo "piece-installer: sign-in failed, attempting first-admin sign-up..."
  curl -sf "$AP_URL/api/v1/authentication/sign-up" \
    -H 'Content-Type: application/json' \
    -d "{\"email\":\"$AP_EMAIL\",\"password\":\"$AP_PASSWORD\",\"firstName\":\"Admin\",\"lastName\":\"User\",\"trackEvents\":false,\"newsLetter\":false}" >/dev/null 2>&1 || true
  RESP=$(sign_in)
  TOKEN=$(printf '%s' "$RESP" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
fi
[ -z "$TOKEN" ] && echo "piece-installer: could not authenticate (check AP_EMAIL/AP_PASSWORD in .env)" && exit 1

INSTALLED=$(curl -sf "$AP_URL/api/v1/pieces" -H "Authorization: Bearer $TOKEN" || true)
if printf '%s' "$INSTALLED" | grep -q "\"name\":\"$PIECE_NAME\".*\"version\":\"$PIECE_VERSION\"\|\"version\":\"$PIECE_VERSION\".*\"name\":\"$PIECE_NAME\""; then
  echo "piece-installer: $PIECE_NAME@$PIECE_VERSION already installed."
  exit 0
fi

echo "piece-installer: installing $PIECE_NAME@$PIECE_VERSION from $TGZ"
curl -sf "$AP_URL/api/v1/pieces" \
  -H "Authorization: Bearer $TOKEN" \
  -F "pieceArchive=@$TGZ" \
  -F "packageType=ARCHIVE" \
  -F "scope=PLATFORM" \
  -F "pieceName=$PIECE_NAME" \
  -F "pieceVersion=$PIECE_VERSION"
echo
echo "piece-installer: done."
