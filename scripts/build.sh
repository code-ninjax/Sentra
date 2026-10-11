#!/usr/bin/env bash
# Build a stripped, reproducible Sentra release binary for Linux.
#
# Usage:
#   bash scripts/build.sh [--pack] [--out-dir DIR] [--goos OS] [--goarch ARCH]
#
# Flags:
#   CGO_ENABLED=0 and -trimpath -ldflags="-s -w" are always applied.
#   --pack additionally produces a UPX-compressed binary (UPX must be
#   installed; `sudo apt install -y upx` on Debian/Ubuntu).
#
# Environment overrides, all optional:
#   GO        Go toolchain to invoke (default: go)
#   GOOS      target OS (default: linux)
#   GOARCH    target architecture (default: amd64)
#   OUT_DIR   artifact directory (default: dist)
#   UPX_BIN   UPX binary to invoke (default: upx)
#
# Artifacts:
#   dist/sentra-<os>-<arch>          stripped, unpacked release binary
#   dist/sentra-<os>-<arch>-packed   same binary, UPX-compressed (--pack only)
#   dist/sizes.txt                   machine-readable sizes and checksums
#
# Budget gates (per project rules: 8-15MB unpacked, 3.5-4.5MB UPX-packed):
#   unpacked over 15 MiB fails the build; packed over 6 MiB fails it.
set -euo pipefail

GO="${GO:-go}"
GOOS="${GOOS:-linux}"
GOARCH="${GOARCH:-amd64}"
OUT_DIR="${OUT_DIR:-dist}"
UPX_BIN="${UPX_BIN:-upx}"
PACK=0

usage() {
  sed -n '2,/^set -euo pipefail$/p' "$0" | sed '$d' | sed 's/^# \{0,1\}//'
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --pack) PACK=1 ;;
    --out-dir) OUT_DIR="${2:?--out-dir needs a directory}"; shift ;;
    --goos) GOOS="${2:?--goos needs an OS}"; shift ;;
    --goarch) GOARCH="${2:?--goarch needs an architecture}"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "error: unknown argument: $1" >&2; exit 2 ;;
  esac
  shift
done

command -v "$GO" >/dev/null 2>&1 || {
  echo "error: Go toolchain '$GO' not found (set GO=/path/to/go)" >&2
  exit 1
}

file_size() {
  if stat -c%s "$1" >/dev/null 2>&1; then
    stat -c%s "$1"
  else
    wc -c < "$1" | tr -d ' '
  fi
}
mib() { awk -v bytes="$1" 'BEGIN { printf "%.2f", bytes / 1048576 }'; }
checksum() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{ print $1 }'
  else
    shasum -a 256 "$1" | awk '{ print $1 }'
  fi
}

mkdir -p "$OUT_DIR"
BIN="$OUT_DIR/sentra-$GOOS-$GOARCH"

echo "building $BIN (CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH, -trimpath -s -w)"
CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" "$GO" build \
  -trimpath \
  -ldflags="-s -w" \
  -o "$BIN" \
  ./cmd/sentra

UNPACKED_BYTES=$(file_size "$BIN")
UNPACKED_MIB=$(mib "$UNPACKED_BYTES")
UNPACKED_SHA=$(checksum "$BIN")
echo "unpacked: $UNPACKED_BYTES bytes ($UNPACKED_MIB MiB)  sha256:$UNPACKED_SHA"

# 15 MiB ceiling: binary size is a first-class constraint.
if [ "$UNPACKED_BYTES" -gt 15728640 ]; then
  echo "error: unpacked binary exceeds the 15 MiB budget ($UNPACKED_MIB MiB); audit new imports" >&2
  exit 1
fi

PACKED_BYTES=0
PACKED_MIB="0.00"
PACKED_SHA=""
if [ "$PACK" -eq 1 ]; then
  command -v "$UPX_BIN" >/dev/null 2>&1 || {
    echo "error: UPX not found ('$UPX_BIN'); install it, e.g. sudo apt install -y upx" >&2
    exit 1
  }
  PACKED_BIN="$BIN-packed"
  cp "$BIN" "$PACKED_BIN"
  "$UPX_BIN" -q "$PACKED_BIN"
  PACKED_BYTES=$(file_size "$PACKED_BIN")
  PACKED_MIB=$(mib "$PACKED_BYTES")
  PACKED_SHA=$(checksum "$PACKED_BIN")
  echo "packed:   $PACKED_BYTES bytes ($PACKED_MIB MiB)  sha256:$PACKED_SHA"
  if [ "$PACKED_BYTES" -gt 6291456 ]; then
    echo "error: packed binary exceeds 6 MiB ($PACKED_MIB MiB); investigate before releasing" >&2
    exit 1
  fi
  if [ "$PACKED_BYTES" -gt 4718592 ]; then
    echo "warning: packed binary above the 4.5 MiB target ($PACKED_MIB MiB)"
  fi
fi

cat > "$OUT_DIR/sizes.txt" <<EOF
unpacked_path=$BIN
unpacked_bytes=$UNPACKED_BYTES
unpacked_mib=$UNPACKED_MIB
unpacked_sha256=$UNPACKED_SHA
packed_bytes=$PACKED_BYTES
packed_mib=$PACKED_MIB
packed_sha256=$PACKED_SHA
EOF
echo "wrote $OUT_DIR/sizes.txt"
