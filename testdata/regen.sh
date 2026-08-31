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
encode base_ip_ref3    "keyint=25:qp=28:ref=3:threads=1:sliced-threads=0:aq-mode=0"
encode base_ip_slices  "keyint=25:qp=28:ref=2:slices=3:threads=1:sliced-threads=0:aq-mode=0"

main() {
  local name=$1 params=$2
  ffmpeg -hide_banner -loglevel error -y     -f rawvideo -pix_fmt yuv420p -s "${W}x${H}" -r 10 -i "$src"     -c:v libx264 -profile:v main -x264-params "$params"     -f h264 "$out/$name.264"
  ffmpeg -hide_banner -loglevel error -y     -i "$out/$name.264" -pix_fmt yuv420p -f rawvideo "$tmp/$name.yuv"
  gzip -9 -c "$tmp/$name.yuv" > "$out/$name.yuv.gz"
}

mcommon='threads=1:sliced-threads=0:aq-mode=0'

main main_intra_cabac      "keyint=1:qp=26:cabac=1:ref=1:bframes=0:$mcommon"
main main_intra_cabac_nodb "keyint=1:qp=26:cabac=1:ref=1:bframes=0:no-deblock=1:$mcommon"
main main_ip_cabac         "keyint=10:qp=26:cabac=1:ref=1:bframes=0:$mcommon"
main main_ipb_cabac        "keyint=10:qp=26:cabac=1:ref=2:bframes=2:b-pyramid=none:$mcommon"
main main_ip_weightp       "keyint=10:qp=26:cabac=1:ref=2:bframes=0:weightp=2:$mcommon"

echo "regenerated $(ls -1 "$out" | wc -l) files in $out"
