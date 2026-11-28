# Niro brand kit

The mark is **D, "In view"**: four corners holding the "in order" square in view. Guardian watching the server, with no mascot, face or eye. Thiago picked it on 2026-11-28 from the four candidates in `docs/design/brand/candidates.html`. The whole kit is on one sheet, `docs/design/brand/kit.html`.

Everything here is built by `build.mjs` from three sources:

- `mark.json`, the mark as 16 rows of 16;
- `vendor/nithron-pixel.json`, the Nithron Pixel face;
- `vendor/icons.json`, the Nithron pixel icons.

To change anything, edit `mark.json` or `build.mjs` and run `node assets/brand/build.mjs` from the repository root. It needs Node, `rsvg-convert` and ImageMagick 7. It rewrites `svg/`, `readme/` and `png/`, and copies the dashboard's icons into `web/public`. Never edit a PNG by hand. The dashboard draws the mark itself (`web/src/components/NiroMark.tsx`), and a test keeps it equal to `mark.json`.

## The grid

- **Grid:** the mark is drawn on a 16 by 16 grid. Its ink fills rows and columns 1 to 14, so the grid itself holds one empty unit on every side.
- **Whole pixels only:** one grid unit is always a whole number of pixels. The sizes are 16, 32, 48, 64, 128, 256 and 512 (1 to 32 px a unit). The avatar uses 20 px a unit, the touch icon 10 and the 192 icon 12. Never scale the mark by a fraction, and never smooth it. Show the PNGs with `image-rendering: pixelated`.
- **Pairing with the name:** Nithron Pixel's capitals are 7 of its 8 units, and the mark's ink is 14 of its 16. So a name at 32 px pairs with the mark at 32 px, and 48 with 48. In the dashboard the mark sits one unit higher than the name's line box, so their tops and bottoms meet.
- **Clear space:** keep at least two grid units clear on every side, beyond the empty unit the grid already holds. At 32 px that is 4 px.

## Colour

- **The mark:** ink only. That is `#131517` on a light ground and `#eceef0` on a dark one (the token values of `--ink`), on a ground of the page's colour (`#f6f7f8` or `#0c0d0f`).
- **Pictures:** colour appears only inside a banner picture (the Discord banner, the social preview, the README pictures), and only from the editor palette of `@nithron/tokens`.
- **Emoji:** one grey, the dark theme's dim (`#84888e`). It reads at 3:1 or more on every Discord ground, from near-black to white. The states are told apart by shape: a filled square for ok, an open square for a warning, a cross for an error.

## Where each file goes

| Use | File | Size |
|---|---|---|
| Discord app icon and bot avatar (Developer Portal) | `png/discord/app-icon-1024.png` (also `avatar-512.png`) | 1024 (512) |
| Discord bot profile banner | `png/discord/profile-banner-1360x480.png` | 680 by 240 at 2x |
| Niro Dev, the development application only | `png/discord/dev-app-icon-1024.png`, `png/discord/dev-profile-banner-1360x480.png` | the mark inverted, and a DEV tag |
| Application emoji (embedded in the binary; see below) | `png/discord/emojis/{niro,shield,lock,ok,warn,error,info}.png` | 128 by 128 |
| Dashboard tab icon | `web/public/favicon.svg` (follows the theme), `favicon.ico` (16, 32, 48) | |
| Home screen icons | `web/public/apple-touch-icon.png` (180), `icon-192.png`, `icon-512.png` | |
| GitHub social preview (repository settings) | `png/social/github-social-preview.png` | 1280 by 640 |
| README pictures | `readme/*-light.svg`, `readme/*-dark.svg` | 846 wide |
| The mark alone | `png/mark/mark-on-{light,dark}-{16..512}.png`, `svg/mark-on-{light,dark}.svg` | |

**The emoji:** the bot uploads its emoji on start, but only those the application does not have yet. So the new emoji replace the old ones on Discord only after the old ones are deleted in the Developer Portal (Emojis) and the bot restarts. That, and uploading the avatar and banner, is done by hand, and only with Thiago's word.

## Never

- Recolour the mark, give it a gradient, a shadow or an outline, or put colour inside it.
- Redraw it off the grid, round its corners, scale it by a fraction or smooth its edges.
- Add a mascot, a face, an eye or a character to it.
- Set it beside Discord's logo or in Discord's blurple, as if Niro were a Discord product.
- Use it in a self-hosted copy: the mark is a trademark (`TRADEMARK_POLICY.md`). The dashboard shows it only where `NIRO_NITHRON_SITE=true`.
- Use the Niro Dev files for the production application.

## Superseded

`superseded/concept-a/` keeps the earlier `<N>` brackets pack (concept A, chosen on 2026-11-22), with its own build script, for the record. Nothing uses it any more.

## Licences

- **Nithron Pixel and the icons:** `vendor/` holds copies of `@nithron/tokens` `fonts/nithron-pixel.json` and `@nithron/icons` 0.2.0 `icons.json`, both under the Nithron Community License v1.0. Replace each file whole; never edit it.
