#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"
out=conformance
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

mkdir -p "$out"

W=176
H=144
src="$tmp/src.yuv"

ffmpeg -hide_banner -loglevel error -y \
  -f lavfi -i "testsrc2=size=${W}x${H}:rate=10:duration=1" \
  -pix_fmt yuv420p -f rawvideo "$src"

encode() {
  local name=$1 params=$2
  ffmpeg -hide_banner -loglevel error -y \
    -f rawvideo -pix_fmt yuv420p -s "${W}x${H}" -r 10 -i "$src" \
    -c:v libx264 -profile:v baseline -x264-params "$params" \
    -f h264 "$out/$name.264"
  ffmpeg -hide_banner -loglevel error -y \
    -i "$out/$name.264" -pix_fmt yuv420p -f rawvideo "$tmp/$name.yuv"
  gzip -9 -c "$tmp/$name.yuv" > "$out/$name.yuv.gz"
}

common='ref=1:threads=1:sliced-threads=0:aq-mode=0'

encode base_intra_qp26 "keyint=1:qp=26:$common"
encode base_intra_nodb "keyint=1:qp=30:no-deblock=1:$common"
encode base_ip_qp10    "keyint=10:qp=10:$common"
encode base_ip_qp26    "keyint=10:qp=26:$common"
encode base_ip_qp40    "keyint=10:qp=40:$common"

echo "regenerated $(ls -1 "$out" | wc -l) files in $out"
