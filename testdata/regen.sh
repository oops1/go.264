#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"
out=conformance
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

mkdir -p "$out"

W=176
H=144
CW=352
CH=288
src="$tmp/src.yuv"
cif="$tmp/cif.yuv"
fade="$tmp/fade.yuv"

ffmpeg -hide_banner -loglevel error -y -f lavfi -i "testsrc2=size=${W}x${H}:rate=10:duration=1" -pix_fmt yuv420p -f rawvideo "$src"
ffmpeg -hide_banner -loglevel error -y -f lavfi -i "testsrc2=size=${W}x${H}:rate=10:duration=1,fade=t=out:st=0:d=1" -pix_fmt yuv420p -f rawvideo "$fade"

ffmpeg -hide_banner -loglevel error -y -f lavfi -i "testsrc2=size=${CW}x${CH}:rate=25:duration=0.32" -pix_fmt yuv420p -f rawvideo "$cif"

cifclip() {
  local name=$1 params=$2
  ffmpeg -hide_banner -loglevel error -y -f rawvideo -pix_fmt yuv420p -s "${CW}x${CH}" -r 25 -i "$cif" -c:v libx264 -profile:v main -x264-params "$params" -f h264 "$out/$name.264"
  ffmpeg -hide_banner -loglevel error -y -i "$out/$name.264" -pix_fmt yuv420p -f rawvideo "$tmp/$name.yuv"
  gzip -9 -c "$tmp/$name.yuv" > "$out/$name.yuv.gz"
}

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

cifclip main_cif_pyramid "keyint=10:qp=26:cabac=1:ref=2:bframes=2:b-pyramid=normal:direct=temporal:$mcommon"

cqm4iy=16,17,18,19,20,21,22,23,24,25,26,27,28,29,30,31
cqm4ic=13,14,15,16,17,18,19,20,21,22,23,24,25,26,27,28
cqm4py=24,23,22,21,20,19,18,17,16,15,14,13,12,11,10,9
cqm8iy=12,13,14,15,16,17,18,19,20,21,22,23,24,25,26,27,28,29,30,31,32,33,34,35,36,37,38,39,40,41,42,43,44,45,46,47,48,49,50,51,52,53,54,55,56,57,58,59,60,61,62,63,64,65,66,67,68,69,70,71,72,73,74,75
cqm8py=40,39,38,37,36,35,34,33,32,31,30,29,28,27,26,25,24,23,22,21,20,19,18,17,16,15,14,13,12,11,10,9,8,7,6,5,4,4,4,4,4,4,4,4,4,4,4,4,4,4,4,4,4,4,4,4,4,4,4,4,4,4,4,4
custom="cqm4iy=$cqm4iy:cqm4ic=$cqm4ic:cqm4py=$cqm4py:cqm8iy=$cqm8iy:cqm8py=$cqm8py"

clip high_intra_8x8_cabac high "keyint=1:qp=26:8x8dct=1:cabac=1:ref=1:bframes=0:$mcommon"
clip high_intra_8x8_cavlc high "keyint=1:qp=26:8x8dct=1:cabac=0:ref=1:bframes=0:$mcommon"
clip high_ip_8x8_cabac    high "keyint=10:qp=26:8x8dct=1:cabac=1:ref=2:bframes=0:weightp=0:$mcommon"
clip high_ip_8x8_cavlc    high "keyint=10:qp=26:8x8dct=1:cabac=0:ref=2:bframes=0:weightp=0:$mcommon"
clip high_ipb_8x8_cabac   high "keyint=10:qp=26:8x8dct=1:cabac=1:ref=2:bframes=2:b-pyramid=none:$mcommon"
clip high_ipb_8x8_cavlc   high "keyint=10:qp=26:8x8dct=1:cabac=0:ref=2:bframes=2:b-pyramid=none:$mcommon"
clip high_cqm_jvt_cabac   high "keyint=10:qp=26:8x8dct=1:cabac=1:ref=2:bframes=2:b-pyramid=none:cqm=jvt:$mcommon"
clip high_cqm_jvt_cavlc   high "keyint=10:qp=26:8x8dct=1:cabac=0:ref=2:bframes=2:b-pyramid=none:cqm=jvt:$mcommon"
clip high_cqm_jvt_4x4     high "keyint=10:qp=26:8x8dct=0:cabac=1:ref=2:bframes=2:b-pyramid=none:cqm=jvt:$mcommon"
clip high_cqm_custom      high "keyint=10:qp=22:8x8dct=1:cabac=1:ref=2:bframes=2:b-pyramid=none:$custom:$mcommon"
clip high_cqm_custom_cavlc high "keyint=10:qp=22:8x8dct=1:cabac=0:ref=2:bframes=2:b-pyramid=none:$custom:$mcommon"

echo "regenerated $(ls -1 "$out" | wc -l) files in $out"
