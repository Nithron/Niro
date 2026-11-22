#!/usr/bin/env bash
# Render every PNG/ICO brand deliverable from the SVG sources in svg/.
# Requires: rsvg-convert (librsvg) and magick (ImageMagick 7).
set -euo pipefail
cd "$(dirname "$0")"

BG='#0B0E13'
S=svg
O=png
rm -rf "$O"
mkdir -p "$O/discord/emojis" "$O/web" "$O/social" "$O/logo"

svg() { rsvg-convert "$@"; }

# Discord: app icon / bot avatar (Discord crops avatars to a circle; the mark sits inside it)
svg -w 1024 -h 1024 "$S/nithro-icon.svg" -o "$O/discord/app-icon-1024.png"
# Discord: profile banner (680x240 display size, exported at 2x)
svg -w 1360 -h 480 "$S/nithro-discord-banner.svg" -o "$O/discord/profile-banner-1360x480.png"
# Discord: application emojis for bot messages (128x128)
for f in "$S"/status/*.svg; do
  svg -w 128 -h 128 "$f" -o "$O/discord/emojis/$(basename "${f%.svg}").png"
done

# Discord: Nithro Dev application (amber "DEV" variant, never used in production)
svg -w 1024 -h 1024 "$S/nithro-dev-icon.svg" -o "$O/discord/dev-app-icon-1024.png"
svg -w 1360 -h 480 "$S/nithro-dev-discord-banner.svg" -o "$O/discord/dev-profile-banner-1360x480.png"
svg -w 32 -h 32 "$S/nithro-dev-icon-small.svg" -o "$O/web/dev-icon-32.png"

# Web / dashboard
cp "$S/nithro-icon.svg" "$O/web/favicon.svg"
# 16-48px use a variant with a larger mark and thicker strokes so it stays crisp
for n in 16 32 48; do
  svg -w $n -h $n "$S/nithro-icon-small.svg" -o "$O/web/icon-$n.png"
done
for n in 192 512; do
  svg -w $n -h $n "$S/nithro-icon.svg" -o "$O/web/icon-$n.png"
done
magick "$O/web/icon-16.png" "$O/web/icon-32.png" "$O/web/icon-48.png" "$O/web/favicon.ico"
# iOS rounds the corners itself, so the touch icon is full-bleed
svg -w 180 -h 180 "$S/nithro-icon.svg" | magick - -background "$BG" -flatten "$O/web/apple-touch-icon.png"

# Social / README
svg -w 1280 -h 640 "$S/nithro-social-preview.svg" -o "$O/social/github-social-preview.png"
svg -w 1200 -h 630 "$S/nithro-og.svg" -o "$O/social/og-image.png"
svg -w 1280 -h 320 "$S/nithro-readme-banner.svg" -o "$O/social/readme-banner.png"

# Logo PNGs (transparent)
for v in dark light; do
  svg -h 216 "$S/nithro-lockup-$v.svg" -o "$O/logo/lockup-$v.png"
  svg -h 432 "$S/nithro-mark-$v.svg" -o "$O/logo/mark-$v.png"
  svg -h 200 "$S/nithro-wordmark-$v.svg" -o "$O/logo/wordmark-$v.png"
done

echo "Rendered $(find "$O" -type f | wc -l) files into $O/"
