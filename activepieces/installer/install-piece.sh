#!/bin/sh
# One-shot piece installer: runs inside the compose network on every `up`,
# exits 0 quickly when the piece version is already installed.
set -eu

apk add --no-cache curl jq >/dev/null 2>&1

PIECE_NAME="${PIECE_NAME:-@activepieces/piece-lake-writer}"
PIECE_VERSION="${PIECE_VERSION:-0.1.2}"

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
if printf '%s' "$INSTALLED" | jq -e --arg name "$PIECE_NAME" --arg version "$PIECE_VERSION" '
  (if type == "array" then . else .data // [] end)[]
  | select(.name == $name and .version == $version)
' >/dev/null; then
  echo "piece-installer: $PIECE_NAME@$PIECE_VERSION already installed."
  exit 0
fi

TGZ=$(ls /piece-dist/*-"$PIECE_VERSION".tgz 2>/dev/null | head -1 || true)
if [ -z "$TGZ" ] && [ -n "${PIECE_ARCHIVE_URL:-}" ]; then
  TGZ="/tmp/lake-writer-$PIECE_VERSION.tgz"
  echo "piece-installer: downloading $PIECE_NAME@$PIECE_VERSION from $PIECE_ARCHIVE_URL"
  curl -fL "$PIECE_ARCHIVE_URL" -o "$TGZ"
fi

if [ -z "$TGZ" ]; then
  echo "piece-installer: $PIECE_NAME@$PIECE_VERSION is not installed and no archive is available."
  echo "piece-installer: bundle piece-archives/*-$PIECE_VERSION.tgz or set JAZ_LAKE_WRITER_ARCHIVE_URL."
  exit 1
fi

echo "piece-installer: installing $PIECE_NAME@$PIECE_VERSION from $TGZ"
curl -sf "$AP_URL/api/v1/pieces" \
  -H "Authorization: Bearer $TOKEN" \
  -F "pieceArchive=@$TGZ" \
  --form-string "packageType=ARCHIVE" \
  --form-string "scope=PLATFORM" \
  --form-string "pieceName=$PIECE_NAME" \
  --form-string "pieceVersion=$PIECE_VERSION"
echo
echo "piece-installer: done."
