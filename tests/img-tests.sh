#!/usr/bin/env bash

set -eu -o pipefail

SCRIPT_DIR="$(dirname "$0")"
CACHE_IMG="${SCRIPT_DIR}/cache-img"
WEB_DIR="${SCRIPT_DIR}/data-web"
NOW="$(date -u "+%s")"

# Hosts from Docker compose file
IMG="linuxfr.org-img"
WEB="nginx"
REDIS="redis"

WEB_HEX="$(printf "%s" "${WEB}"|xxd -ps)"
# shellcheck disable=SC2034
TARGET4="$(dig "${IMG}" A +short)" # img IPv4
# shellcheck disable=SC2034
TARGET6="[$(dig "${IMG}" AAAA +short)]" # img IPv6
WEB4="$(dig "${WEB}" A +short)"
WEB4_HEX="$(printf "%s" "${WEB4}"|xxd -ps)"
WEB6="[$(dig "${WEB}" AAAA +short)]"
WEB6_HEX="$(printf "%s" "${WEB6}"|xxd -ps)"

REDIS_CLI=(docker exec -i "tests-${REDIS}-1" redis-cli)
# without docker REDIS_CLI=(redis-cli -p 16379)
HURL=(hurl)
SANITY=(docker exec -i "tests-${IMG}-1" /app/img-LinuxFr.org -r "${REDIS}:6379/0" -d cache -l - -c)

IMAGES_WITH_ONLY_IMG_ENTRY_NO_CACHE="http://bad${WEB}.example.net/nowhere
http://bad${WEB}/nowhere
http://${WEB}:81/closed_port
http://${WEB}/redirectloop"
IMAGES_WITH_ONLY_IMG_ENTRY_NO_CACHE_AND_BLOCKED="http://${WEB}/blocked.png"
IMAGES_WITH_ONLY_IMG_ENTRY_STILL_IN_CACHE="http://${WEB}/red_100x100_blocked_after_fetch.png"

IMAGES_WITH_IMG_AND_ERR_ENTRIES_NO_CACHE="http://${WEB}/bad_content.html
http://${WEB}/bad_content.php
http://${WEB}/bad_content.sh
http://${WEB}/bad_content.txt
http://${WEB}/forbidden.png
http://${WEB}/non_existing
http://${WEB}/random_2000x2000.png
http://${WEB}/status400
http://${WEB}/status401
http://${WEB}/status409
http://${WEB}/status410
http://${WEB}/status412
http://${WEB}/status415
http://${WEB}/status422
http://${WEB}/status429
http://${WEB}/status436
http://${WEB}/status441
http://${WEB}/status500
http://${WEB}/status501
http://${WEB}/status502
http://${WEB}/status503
http://${WEB}/status504
http://${WEB}/status520
http://${WEB}/status525
http://${WEB}/status530
http://${WEB}/status666"
IMAGES_WITH_IMG_AND_ERR_ENTRIES_STILL_IN_CACHE="http://${WEB}/red_100x100_removed_after_fetch.png"

IMAGES_WITH_IMG_AND_UPDATED_ENTRIES="http://${WEB}/red_10000x10000.png
http://${WEB}/red_100x100.avif
http://${WEB}/red_100x100.bmp
http://${WEB}/red_100x100.gif
http://${WEB}/red_100x100.jpeg
http://${WEB}/red_100x100.jpg
http://${WEB}/red_100x100.png
http://${WEB}/red_100x100_changed_after_fetch.png
http://${WEB}/red_100x100_converted_after_fetch.png
http://${WEB}/red_100x100.svg
http://${WEB}/red_100x100.tiff
http://${WEB}/red_100x100.webp
http://${WEB4}/blue_100x100.png
http://${WEB6}/green_100x100.png
http://${WEB}/status301
http://${WEB}/status302
http://${WEB}/extraname
http://${WEB}/extrafield
http://${WEB}/status308"

printf "Prepare/restore images altered after first fetch\n"
cp "${WEB_DIR}/red_100x100.png" "${WEB_DIR}/red_100x100_removed_after_fetch.png"
cp "${WEB_DIR}/red_100x100.png" "${WEB_DIR}/red_100x100_changed_after_fetch.png"
cp "${WEB_DIR}/red_100x100.png" "${WEB_DIR}/red_100x100_converted_after_fetch.png"
cp "${WEB_DIR}/red_100x100.png" "${WEB_DIR}/red_100x100_blocked_after_fetch.png"

IMAGES="$IMAGES_WITH_IMG_AND_UPDATED_ENTRIES
$IMAGES_WITH_ONLY_IMG_ENTRY_NO_CACHE
$IMAGES_WITH_ONLY_IMG_ENTRY_NO_CACHE_AND_BLOCKED
$IMAGES_WITH_ONLY_IMG_ENTRY_STILL_IN_CACHE
$IMAGES_WITH_IMG_AND_ERR_ENTRIES_NO_CACHE
$IMAGES_WITH_IMG_AND_ERR_ENTRIES_STILL_IN_CACHE"

printf "Cleaning img cache directory: %s\n" "$CACHE_IMG"
rm -rf -- "$CACHE_IMG"/[0-9a-f][0-9a-f]

printf "Preload tests images in Redis\n"
for img in $IMAGES
do
"${REDIS_CLI[@]}" > /dev/null <<EOF
DEL img/$img img/updated/$img img/err/$img img/latest img/blocked
HSET img/$img created_at $NOW
LPUSH img/latest $img
EOF
done
for img in $IMAGES_WITH_ONLY_IMG_ENTRY_NO_CACHE_AND_BLOCKED
do
"${REDIS_CLI[@]}" > /dev/null <<EOF
HSET img/$img status Blocked
LPUSH img/blocked $img
EOF
done

hurl_tests()
{
  for ip in 4 6
  do
    for http2 in false true
    do
      target="TARGET$ip"
      printf "Testing with IPv%s HTTP/2 %s\n" "${ip}" "${http2}"
      "${HURL[@]}" -$ip ${DEBUG:+-v} \
        --variable "TARGET=${!target}" \
        --variable "HTTP2=${http2}" \
        --variable "WEB=${WEB}" \
        --variable "WEB_HEX=${WEB_HEX}" \
        --variable "WEB4_HEX=${WEB4_HEX}" \
        --variable "WEB4=${WEB4}" \
        --variable "WEB4_HEX=${WEB4_HEX}" \
        --variable "WEB6=${WEB6}" \
        --variable "WEB6_HEX=${WEB6_HEX}" \
        --test "$@"
    done
  done
}

# tests first fetch
hurl_tests tests_misc.hurl tests_img.hurl tests_avatars.hurl

# alter images after first fetch
cp "${WEB_DIR}/red_10000x10000.png" "${WEB_DIR}/red_100x100_changed_after_fetch.png"
rm "${WEB_DIR}/red_100x100_removed_after_fetch.png"
cp "${WEB_DIR}/red_100x100.gif" "${WEB_DIR}/red_100x100_converted_after_fetch.png"
"${REDIS_CLI[@]}" HSET img/http://${WEB}/red_100x100_blocked_after_fetch.png status Blocked
"${REDIS_CLI[@]}" LPUSH img/blocked http://${WEB}/red_100x100_blocked_after_fetch.png > /dev/null

# tests after first fetch but before cache expiration
hurl_tests tests_img_after_fetch_before_cache_expiration.hurl

# alter images after first fetch to trigger cache expiration
for img in \
"http://${WEB}/red_100x100_blocked_after_fetch.png" \
"http://${WEB}/red_100x100_changed_after_fetch.png" \
"http://${WEB}/red_100x100_converted_after_fetch.png" \
"http://${WEB}/red_100x100_removed_after_fetch.png"
do
"${REDIS_CLI[@]}" > /dev/null <<EOF
DEL img/updated/$img
EOF
done

# tests after first fetch but after cache expiration
hurl_tests tests_img_after_fetch_and_cache_expiration.hurl

# 1 counted things / 2 computed / 3 expected
check() {
  if [ "$2" != "$3" ] ; then
    printf "*KO* Unexpected number of %s (%d instead of %d)\n" "$1" "$2" "$3"
    exit 1
  else
    printf "OK Expected number of %s (%d)\n" "$1" "$2"
  fi
}

REDIS_IMG_ERR="$("${REDIS_CLI[@]}" KEYS img/err/*|wc -l)"
REDIS_IMG_ERR_EXPECTED_NO_CACHE="$(printf "%s\n" "$IMAGES_WITH_IMG_AND_ERR_ENTRIES_NO_CACHE"|wc -l)"
REDIS_IMG_ERR_EXPECTED_STILL_IN_CACHE="$(printf "%s\n" "$IMAGES_WITH_IMG_AND_ERR_ENTRIES_STILL_IN_CACHE"|wc -l)"
check "img/err" "$REDIS_IMG_ERR" "$(( REDIS_IMG_ERR_EXPECTED_NO_CACHE + REDIS_IMG_ERR_EXPECTED_STILL_IN_CACHE ))"

REDIS_IMG_UPDATED="$("${REDIS_CLI[@]}" KEYS img/updated/*|wc -l)"
REDIS_IMG_UPDATED_EXPECTED="$(printf "%s\n" "$IMAGES_WITH_IMG_AND_UPDATED_ENTRIES"|wc -l)"
check "img/updated" "$REDIS_IMG_UPDATED" "$REDIS_IMG_UPDATED_EXPECTED"

REDIS_IMG_URI="$("${REDIS_CLI[@]}" KEYS img/h*|wc -l)"
REDIS_IMG_URI_ONLY_NO_CACHE="$(printf "%s\n" "$IMAGES_WITH_ONLY_IMG_ENTRY_NO_CACHE"|wc -l)"
REDIS_IMG_URI_ONLY_NO_CACHE_AND_BLOCKED="$(printf "%s\n" "$IMAGES_WITH_ONLY_IMG_ENTRY_NO_CACHE_AND_BLOCKED"|wc -l)"
REDIS_IMG_URI_ONLY_STILL_IN_CACHE="$(printf "%s\n" "$IMAGES_WITH_ONLY_IMG_ENTRY_STILL_IN_CACHE"|wc -l)"
REDIS_IMG_URI_EXPECTED="$(( REDIS_IMG_ERR_EXPECTED_NO_CACHE + REDIS_IMG_ERR_EXPECTED_STILL_IN_CACHE + REDIS_IMG_UPDATED_EXPECTED + REDIS_IMG_URI_ONLY_NO_CACHE + REDIS_IMG_URI_ONLY_NO_CACHE_AND_BLOCKED + REDIS_IMG_URI_ONLY_STILL_IN_CACHE ))"
check "img/<uri>" "$REDIS_IMG_URI" "$REDIS_IMG_URI_EXPECTED"

REDIS_ALL="$("${REDIS_CLI[@]}" DBSIZE)"
REDIS_IMG_LATEST=1
REDIS_IMG_BLOCKED=1
REDIS_ALL_EXPECTED="$(( 2 * REDIS_IMG_ERR_EXPECTED_NO_CACHE + 2 * REDIS_IMG_ERR_EXPECTED_STILL_IN_CACHE + 2 * REDIS_IMG_UPDATED_EXPECTED + REDIS_IMG_URI_ONLY_NO_CACHE + REDIS_IMG_URI_ONLY_NO_CACHE_AND_BLOCKED + REDIS_IMG_URI_ONLY_STILL_IN_CACHE + REDIS_IMG_LATEST + REDIS_IMG_BLOCKED ))"
check "keys" "$REDIS_ALL" "$REDIS_ALL_EXPECTED"

CACHE_ENTRIES="$(find cache-img -type f|wc -l)"
check "cache entries" "$CACHE_ENTRIES" "$(( REDIS_IMG_UPDATED + REDIS_IMG_ERR_EXPECTED_STILL_IN_CACHE + REDIS_IMG_URI_ONLY_STILL_IN_CACHE ))"

printf "Cleanup before sanity check, ie. remove broken test images\n"
for img in $IMAGES_WITH_ONLY_IMG_ENTRY_NO_CACHE
do
"${REDIS_CLI[@]}" > /dev/null <<EOF
DEL img/$img
LREM "img/latest" 1 "$img"
EOF
done
for img in $IMAGES_WITH_ONLY_IMG_ENTRY_NO_CACHE_AND_BLOCKED
do
"${REDIS_CLI[@]}" > /dev/null <<EOF
DEL img/$img
LREM "img/latest" 1 "$img"
LREM "img/blocked" 1 "$img"
EOF
done
for img in $IMAGES_WITH_IMG_AND_ERR_ENTRIES_NO_CACHE
do
"${REDIS_CLI[@]}" > /dev/null <<EOF
DEL img/$img img/err/$img
LREM "img/latest" 1 "$img"
EOF
done
for img in $IMAGES_WITH_ONLY_IMG_ENTRY_STILL_IN_CACHE
do
"${REDIS_CLI[@]}" > /dev/null <<EOF
HDEL img/$img status
LREM "img/blocked" 1 "$img"
EOF
done

printf "Sanity check"
"${SANITY[@]}"

printf "All tests look good!\n"
