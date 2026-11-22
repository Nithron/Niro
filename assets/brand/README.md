# Nithro brand assets

The logo is the `<N>` mark: Nithron's angle brackets around an N, in the blue→mint Nithro gradient.
SVG sources live in `svg/`. Every PNG/ICO in `png/` is generated from them; after editing an SVG, run
`./build.sh` (needs `rsvg-convert` and ImageMagick 7).

## Where each file goes

| Use | File | Size |
|---|---|---|
| Discord app icon + bot avatar (Developer Portal) | `png/discord/app-icon-1024.png` | 1024×1024 |
| Discord bot profile banner | `png/discord/profile-banner-1360x480.png` | 680×240 @2x |
| **Nithro Dev** app icon / banner (development only) | `png/discord/dev-app-icon-1024.png`, `png/discord/dev-profile-banner-1360x480.png` | amber `<N>` with a DEV tag |
| Discord application emojis (Guardian alerts etc.) | `png/discord/emojis/*.png` (`nithro`, `shield`, `lock`, `ok`, `warn`, `error`, `info`) | 128×128 |
| Dashboard favicon | `png/web/favicon.svg`, `favicon.ico` (16/32/48), `icon-16/32/48.png` | |
| PWA / Android | `png/web/icon-192.png`, `icon-512.png` | |
| iOS home screen | `png/web/apple-touch-icon.png` (full-bleed; iOS rounds it) | 180×180 |
| GitHub repo social preview (Settings → Social preview) | `png/social/github-social-preview.png` | 1280×640 |
| Link previews (`og:image`) | `png/social/og-image.png` | 1200×630 |
| README header | `png/social/readme-banner.png` | 1280×320 |
| Logo on dark / light backgrounds | `svg/nithro-lockup-{dark,light}.svg`, `svg/nithro-mark-{dark,light}.svg`, `svg/nithro-wordmark-{dark,light}.svg` (PNG copies in `png/logo/`) | |

The **dev variant** (`svg/nithro-dev-*.svg`) swaps the gradient to amber `#FFC24B` → `#FF7A45` and adds faint hazard stripes and a DEV tag, so a development bot is never mistaken for production. Never use it for the production app.

`svg/nithro-icon-small.svg` is the 16–48px variant: a larger mark with thicker strokes, so it stays crisp in browser tabs.

## Colours (`palette.json`)

| Token | Hex | Use |
|---|---|---|
| Background | `#0B0E13` | Icon tile, dark surfaces (shared with NithronOS) |
| Surface | `#121622` | Cards on dark |
| Text | `#E6EDF7` | The N and the wordmark on dark |
| Text on light | `#121418` | The N and the wordmark on light |
| Muted | `#9AA7B8` | Taglines, secondary text |
| Gradient (dark bg) | `#4CB8FF` → `#3DDBA6` | Brackets |
| Gradient (light bg) | `#0B8FE0` → `#0EA57D` | Brackets (darker, for contrast on white). These are nithron.com's brand blue and mint |
| Success / Warning / Danger / Info | `#3DDBA6` / `#F5B83D` / `#FF6B6B` / `#4CB8FF` | Status emojis, dashboard states |

## Rules

- Use the `-dark` files on dark backgrounds and the `-light` files on light backgrounds. Don't recolour the gradient.
- Leave clear space around the mark at least the width of one bracket stroke.
- The wordmark is drawn as vector strokes rather than typed, so it never depends on an installed font.
  Taglines on the social images use Mona Sans (nithron.com's font) with Inter or system-sans fallbacks.
- Don't place the mark next to Discord's logo or use Discord's blurple in ways that suggest Nithro is a Discord product.
- `concepts/` keeps the three original concepts for reference. Concept A was chosen on 2026-11-22.
