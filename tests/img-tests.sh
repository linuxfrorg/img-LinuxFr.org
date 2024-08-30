#!/usr/bin/env bash

set -eu -o pipefail

SCRIPT_DIR="$(dirname "$0")"
CACHE_IMG="${SCRIPT_DIR}/cache-img"
NOW="$(date -u "+%s")"

# IPs from Docker compose file
TARGET4="192.168.42.40"        # img IPv4
TARGET6="[fd42:3200:3200::40]" # img IPv6
NGINX4="192.168.42.20"
NGINX4_HEX="$(printf "%s" "${NGINX4}"|xxd -ps)"
NGINX6="[fd42:3200:3200::20]"
NGINX6_HEX="$(printf "%s" "${NGINX6}"|xxd -ps)"

REDIS_CLI=(docker exec -i tests_redis_1 redis-cli)
# without docker REDIS_CLI=(redis-cli -p 16379)
HURL=(hurl)

IMAGES_WITH_ONLY_IMG_ENTRY="http://badnginx.example.net/nowhere
http://badnginx/nowhere
http://nginx:81/closed_port
http://nginx/redirectloop
http://nginx/blocked.png
http://nginx/red_100x100_blocked_after_fetch.png"

IMAGES_WITH_IMG_AND_ERR_ENTRIES="http://nginx/bad_content.html
http://nginx/bad_content.php
http://nginx/bad_content.sh
http://nginx/bad_content.txt
http://nginx/forbidden.png
http://nginx/non_existing
http://nginx/random_2000x2000.png
http://nginx/status400
http://nginx/status401
http://nginx/status409
http://nginx/status410
http://nginx/status412
http://nginx/status415
http://nginx/status422
http://nginx/status429
http://nginx/status436
http://nginx/status441
http://nginx/status500
http://nginx/status501
http://nginx/status502
http://nginx/status503
http://nginx/status504
http://nginx/status520
http://nginx/status525
http://nginx/status530
http://nginx/status666
http://nginx/red_100x100_removed_after_fetch.png"

IMAGES_WITH_IMG_AND_UPDATED_ENTRIES="http://nginx/red_10000x10000.png
http://nginx/red_100x100.avif
http://nginx/red_100x100.bmp
http://nginx/red_100x100.gif
http://nginx/red_100x100.jpeg
http://nginx/red_100x100.jpg
http://nginx/red_100x100.png
http://nginx/red_100x100_changed_after_fetch.png
http://nginx/red_100x100_converted_after_fetch.png
http://nginx/red_100x100.svg
http://nginx/red_100x100.tiff
http://nginx/red_100x100.webp
http://${NGINX4}/blue_100x100.png
http://${NGINX6}/green_100x100.png
http://nginx/status301
http://nginx/status302
http://nginx/status308"

printf "Prepare/restore images altered after first fetch\n"
cp data-nginx/red_100x100.png data-nginx/red_100x100_removed_after_fetch.png
cp data-nginx/red_100x100.png data-nginx/red_100x100_changed_after_fetch.png
cp data-nginx/red_100x100.png data-nginx/red_100x100_converted_after_fetch.png

IMAGES="$IMAGES_WITH_IMG_AND_UPDATED_ENTRIES
$IMAGES_WITH_ONLY_IMG_ENTRY
$IMAGES_WITH_IMG_AND_ERR_ENTRIES"

printf "Cleaning img cache directory: %s\n" "$CACHE_IMG"
rm -rf -- "$CACHE_IMG"/[0-9a-f][0-9a-f]

printf "Preload tests images in Redis\n"
for img in $IMAGES
do
${REDIS_CLI[@]} > /dev/null <<EOF
del img/$img img/updated/$img img/err/$img
hset img/$img created_at $NOW
EOF
done
${REDIS_CLI[@]} hset img/http://nginx/blocked.png status Blocked > /dev/null

# tests first fetch
for ip in 4 6
do
  for http2 in false true
  do
    target="TARGET$ip"
    printf "Testing with IPv%s HTTP/2 %s\n" "${ip}" "${http2}"
    ${HURL[@]} -$ip ${DEBUG:+-v} \
      --variable "TARGET=${!target}" \
      --variable "HTTP2=${http2}" \
      --variable "NGINX4=$NGINX4" \
      --variable "NGINX4_HEX=$NGINX4_HEX" \
      --variable "NGINX6=$NGINX6" \
      --variable "NGINX6_HEX=$NGINX6_HEX" \
      --test tests_misc.hurl tests_img.hurl tests_avatars.hurl
  done
done

# alter images after first fetch
cp data-nginx/red_10000x10000.png data-nginx/red_100x100_changed_after_fetch.png
rm data-nginx/red_100x100_removed_after_fetch.png
cp data-nginx/red_100x100.gif data-nginx/red_100x100_converted_after_fetch.png
${REDIS_CLI[@]} hset img/http://nginx/red_100x100_blocked_after_fetch.png status Blocked

# tests after first fetch

for ip in 4 6
do
  for http2 in false true
  do
    target="TARGET$ip"
    printf "Testing with IPv%s HTTP/2 %s\n" "${ip}" "${http2}"
    ${HURL[@]} -$ip ${DEBUG:+-v} \
      --variable "TARGET=${!target}" \
      --variable "HTTP2=${http2}" \
      --variable "NGINX4=$NGINX4" \
      --variable "NGINX4_HEX=$NGINX4_HEX" \
      --variable "NGINX6=$NGINX6" \
      --variable "NGINX6_HEX=$NGINX6_HEX" \
      --test tests_img_after_fetch.hurl
  done
done

# 1 counted things / 2 computed / 3 expected
check() {
  if [ "$2" != "$3" ] ; then
    printf "*KO* Unexpected number of %s (%d instead of %d)\n" "$1" "$2" "$3"
    exit 1
  else
    printf "OK Expected number of %s (%d)\n" "$1" "$2"
  fi
}

REDIS_IMG_ERR="$(${REDIS_CLI[@]} keys img/err/*|wc -l)"
REDIS_IMG_ERR_EXPECTED="$(printf "%s\n" "$IMAGES_WITH_IMG_AND_ERR_ENTRIES"|wc -l)"
check "img/err" "$REDIS_IMG_ERR" "$REDIS_IMG_ERR_EXPECTED"

REDIS_IMG_UPDATED="$(${REDIS_CLI[@]} keys img/updated/*|wc -l)"
REDIS_IMG_UPDATED_EXPECTED="$(printf "%s\n" "$IMAGES_WITH_IMG_AND_UPDATED_ENTRIES"|wc -l)"
check "img/updated" "$REDIS_IMG_UPDATED" "$REDIS_IMG_UPDATED_EXPECTED"

REDIS_IMG_URI="$(${REDIS_CLI[@]} keys img/h*|wc -l)"
REDIS_IMG_URI_ONLY="$(printf "%s\n" "$IMAGES_WITH_ONLY_IMG_ENTRY"|wc -l)"
REDIS_IMG_URI_EXPECTED="$(( REDIS_IMG_ERR_EXPECTED + REDIS_IMG_UPDATED_EXPECTED + REDIS_IMG_URI_ONLY))"
check "img/<uri>" "$REDIS_IMG_URI" "$REDIS_IMG_URI_EXPECTED"

REDIS_ALL="$(${REDIS_CLI[@]} dbsize)"
REDIS_ALL_EXPECTED="$(( REDIS_IMG_ERR_EXPECTED + REDIS_IMG_UPDATED_EXPECTED + REDIS_IMG_URI_EXPECTED ))"
check "keys" "$REDIS_ALL" "$REDIS_ALL_EXPECTED"

CACHE_ENTRIES="$(find cache-img -type f|wc -l)"
check "cache entries" "$CACHE_ENTRIES" "$REDIS_IMG_UPDATED"

printf "All tests looks good!\n"
