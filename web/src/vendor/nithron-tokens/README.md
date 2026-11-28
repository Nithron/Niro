# Nithron tokens (vendored)

`line.css` is `dist/line.css` of `@nithron/tokens` 0.4.0, from the nithron-site repository (packages/tokens). It is copied whole: its first line names the package and the version, and only that line differs from the package's file. Never edit it. To update, copy the new `dist/line.css` over it and put the new version in its first line. `src/test/identity.test.ts` checks that the line is there.

The package is under the Nithron Community License v1.0, the same licence text as this repository's `LICENSE` (the two files are byte-identical), so no separate licence file is kept here.

Niro maps its own variable names onto these tokens in `src/styles.css`. Components use Niro's names, never the tokens' values.
