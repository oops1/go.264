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
fade="$tmp/fade.yuv"

ffmpeg -hide_banner -loglevel error -y -f lavfi -i "testsrc2=size=${W}x${H}:rate=10:duration=1" -pix_fmt yuv420p -f rawvideo "$src"
ffmpeg -hide_banner -loglevel error -y -f lavfi -i "testsrc2=size=${W}x${H}:rate=10:duration=1,fade=t=out:st=0:d=1" -pix_fmt yuv420p -f rawvideo "$fade"

clip() {
  local name=$1 profile=$2 params=$3 input=${4:-$src}
  ffmpeg -hide_banner -loglevel error -y -f rawvideo -pix_fmt yuv420p -s "${W}x${H}" -r 10 -i "$input" -c:v libx264 -profile:v "$profile" -x264-params "$params" -f h264 "$out/$name.264"
  ffmpeg -hide_banner -loglevel error -y -i "$out/$name.264" -pix_fmt yuv420p -f rawvideo "$tmp/$name.yuv"
  gzip -9 -c "$tmp/$name.yuv" > "$out/$name.yuv.gz"
}

common='ref=1:threads=1:sliced-threads=0:aq-mode=0'
mcommon='threads=1:sliced-threads=0:aq-mode=0'

clip base_intra_qp26 baseline "keyint=1:qp=26:$common"
clip base_intra_nodb baseline "keyint=1:qp=30:no-deblock=1:$common"
clip base_ip_qp10    baseline "keyint=10:qp=10:$common"
clip base_ip_qp26    baseline "keyint=10:qp=26:$common"
clip base_ip_qp40    baseline "keyint=10:qp=40:$common"
clip base_ip_ref3    baseline "keyint=25:qp=28:ref=3:$mcommon"
clip base_ip_slices  baseline "keyint=25:qp=28:ref=2:slices=3:$mcommon"

clip main_intra_cabac      main "keyint=1:qp=26:cabac=1:ref=1:bframes=0:$mcommon"
clip main_intra_cabac_nodb main "keyint=1:qp=26:cabac=1:ref=1:bframes=0:no-deblock=1:$mcommon"
clip main_ip_cabac         main "keyint=10:qp=26:cabac=1:ref=1:bframes=0:weightp=0:$mcommon"
clip main_ipb_cabac        main "keyint=10:qp=26:cabac=1:ref=2:bframes=2:b-pyramid=none:weightp=0:weightb=0:$mcommon"
clip main_ipb_cavlc        main "keyint=10:qp=26:cabac=0:ref=2:bframes=2:b-pyramid=none:weightp=0:weightb=0:$mcommon"
clip main_ipb_temporal     main "keyint=10:qp=26:cabac=1:ref=2:bframes=2:b-pyramid=none:weightp=0:weightb=0:direct=temporal:$mcommon"
clip main_ipb_weightb      main "keyint=10:qp=26:cabac=1:ref=2:bframes=2:b-pyramid=none:weightp=0:weightb=1:$mcommon"
clip main_ipb_pyramid      main "keyint=25:qp=28:cabac=1:ref=3:bframes=3:b-pyramid=normal:$mcommon"
clip main_ip_weightp       main "keyint=10:qp=26:cabac=1:ref=2:bframes=0:weightp=2:$mcommon"
clip main_ip_fade          main "keyint=10:qp=26:cabac=1:ref=2:bframes=0:weightp=2:$mcommon" "$fade"

echo "regenerated $(ls -1 "$out" | wc -l) files in $out"
