# Fonts

| File | Face | Source | Licence |
|---|---|---|---|
| `MonaSansVF-opsz-wght.woff2` | Mona Sans, variable (weight 200 to 900, optical size) | github.com/github/mona-sans, release v2.0.27, `mona-sans-webfonts-v2.0.27.zip`, `fonts/webfonts/variable/MonaSansVF[opsz,wght].woff2` | SIL Open Font License 1.1, `public/fonts/MonaSans-OFL.txt` |
| `../../../public/fonts/nithron-pixel.woff2` | Nithron Pixel, the pixel display face (5 by 7 grid, ASCII and 22 marks, no accented letters) | nithron-site, packages/tokens/fonts (with tokens 0.4.0); kept in public/fonts because Vite would inline a 2 KB file as a data: URL, which the CSP refuses | Nithron Community License v1.0, the same text as this repository's `LICENSE` |
| `MonaspaceNeonVar.woff2` | Monaspace Neon, variable (weight 200 to 800) | github.com/githubnext/monaspace, release v1.400, `monaspace-webfont-variable-v1.400.zip`, `Variable Web Fonts/Monaspace Neon/Monaspace Neon Var.woff2` | SIL Open Font License 1.1, `public/fonts/Monaspace-LICENSE.txt` |

Mona Sans and Monaspace Neon are the upstream files unchanged (only renamed), so the reserved font names stay with unmodified fonts. Do not subset or convert them. The licence texts sit in `public/fonts`, so every build carries them next to the fonts. `styles.css` declares the faces under the names the Nithron tokens use ("Mona Sans Variable", "Monaspace Neon").

Nithron Pixel is sharp only at whole multiples of 8 px (16, 24, 32, 48) and standing on whole pixels; use the `.pixel` classes in `styles.css`, never another size.
