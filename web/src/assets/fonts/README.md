# Fonts

| File | Face | Source | Licence |
|---|---|---|---|
| `MonaSansVF-opsz-wght.woff2` | Mona Sans, variable (weight 200 to 900, optical size) | github.com/github/mona-sans, release v2.0.27, `mona-sans-webfonts-v2.0.27.zip`, `fonts/webfonts/variable/MonaSansVF[opsz,wght].woff2` | SIL Open Font License 1.1, `public/fonts/MonaSans-OFL.txt` |
| `MonaspaceNeonVar.woff2` | Monaspace Neon, variable (weight 200 to 800) | github.com/githubnext/monaspace, release v1.400, `monaspace-webfont-variable-v1.400.zip`, `Variable Web Fonts/Monaspace Neon/Monaspace Neon Var.woff2` | SIL Open Font License 1.1, `public/fonts/Monaspace-LICENSE.txt` |

Both files are the upstream files unchanged (only renamed), so the reserved font names stay with unmodified fonts. Do not subset or convert them. The licence texts sit in `public/fonts`, so every build carries them next to the fonts. `styles.css` declares the faces under the names the Nithron tokens use ("Mona Sans Variable", "Monaspace Neon").
