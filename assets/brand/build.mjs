// Builds Niro's brand kit from the mark (mark.json), the Nithron Pixel face and the Nithron pixel icons (vendor/).
//   node assets/brand/build.mjs
// Writes svg/ and readme/ (SVG sources), renders png/ from them (rsvg-convert and ImageMagick 7), and copies the
// dashboard's icons into web/public. Every picture is whole pixels: each grid unit is a whole number of output pixels.
import { readFileSync, writeFileSync, mkdirSync, rmSync, copyFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { execFileSync } from 'node:child_process';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..', '..');
const read = (p) => JSON.parse(readFileSync(join(here, p), 'utf8'));
const MARK = read('mark.json').rows;
const FONT = read('vendor/nithron-pixel.json');
const ICONS = read('vendor/icons.json').icons;
const ADVANCE = FONT.grid.advance;

// The token values of the soft grounds (@nithron/tokens line.css) and the editor palette, the only colours used.
const THEMES = {
  dark: { page: '#0c0d0f', plate: '#101113', bar: '#191b1e', hair: '#34373c', ink: '#eceef0', ink2: '#a9adb3', dim: '#84888e', blue: '#74acff', cyan: '#6fd2fc', gold: '#f0c551', orange: '#ff963d', violet: '#9c9afe', green: '#54dd7d' },
  light: { page: '#f6f7f8', plate: '#fbfbfc', bar: '#eaecef', hair: '#c2c5ca', ink: '#131517', ink2: '#4c5056', dim: '#62666c', blue: '#1f5dc2', cyan: '#0371a0', gold: '#916304', orange: '#c04502', violet: '#5a43d4', green: '#017b37' },
};
const D = THEMES.dark, L = THEMES.light;
// Emoji sit on Discord's grounds, from near black to white: the dark theme's dim grey reads at 3:1 or more on all of them.
const EMOJI_INK = D.dim;

// ---- drawing ------------------------------------------------------------------------------------------------
const glyph = (c) => FONT.glyphs[FONT.same?.[c] ?? c] ?? FONT.glyphs[FONT.notdef] ?? FONT.glyphs['?'];
const runs = (rows, x0, y0, s) => rows.map((row, y) => [...row.matchAll(/#+/g)].map((m) => 'M' + (x0 + m.index * s) + ' ' + (y0 + y * s) + 'h' + m[0].length * s + 'v' + s + 'h-' + m[0].length * s + 'z').join('')).join('');
const text = (str, x, y, s, fill) => '<path fill="' + fill + '" d="' + [...str].map((c, i) => runs(glyph(c), x + i * ADVANCE * s, y, s)).join('') + '"/>';
const textWidth = (str, s) => (str.length * ADVANCE - 1) * s;
const icon = (name, x, y, s, fill) => { const i = ICONS[name]; if (!i) throw new Error('no icon named ' + name); return '<path fill="' + fill + '" d="' + runs(i.rows, x, y, s) + '"/>'; };
const mark = (x, y, s, fill) => '<path fill="' + fill + '" d="' + runs(MARK, x, y, s) + '"/>';
const rect = (x, y, w, h, fill) => '<rect x="' + x + '" y="' + y + '" width="' + w + '" height="' + h + '" fill="' + fill + '"/>';
const frame = (x, y, w, h, line, t = 2) => rect(x, y, w, t, line) + rect(x, y + h - t, w, t, line) + rect(x, y, t, h, line) + rect(x + w - t, y, t, h, line);
const esc = (s) => s.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;');
const svg = (w, h, body, label, style = '') => '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ' + w + ' ' + h + '" width="' + w + '" height="' + h + '" shape-rendering="crispEdges" role="img" aria-label="' + esc(label) + '">' + (style ? '<style>' + style + '</style>' : '') + body + '</svg>\n';

/** The mark's four corners, drawn around any box: each an L of a 4 by 2 arm and a 2 by 4 arm, in units of s. */
const corners = (x, y, w, h, s, fill) => {
  const a = 4 * s, b = 2 * s;
  return [[x, y, 1, 1], [x + w, y, -1, 1], [x, y + h, 1, -1], [x + w, y + h, -1, -1]].map(([cx, cy, dx, dy]) => {
    const hx = dx > 0 ? cx : cx - a, hy = dy > 0 ? cy : cy - b;
    const vx = dx > 0 ? cx : cx - b, vy = dy > 0 ? cy : cy - a;
    return rect(hx, hy, a, b, fill) + rect(vx, vy, b, a, fill);
  }).join('');
};

/**
 * The picture of the banners: a few messages of a conversation as bars, one of them carrying a link that is held in
 * view by the mark's corners. Units of s px; 80 by 56 units.
 */
const CHAT = [
  { name: 12, text: [[62, 'hair']] },
  { name: 16, text: [[44, 'hair']] },
  { name: 10, text: [[26, 'hair'], [34, 'orange']], held: true },
  { name: 14, text: [[72, 'hair']] },
  { name: 10, text: [[50, 'hair']] },
];
const CHAT_W = 80;
const chat = (t, x, y, s) => CHAT.map((r, i) => {
  const ry = y + i * 12 * s;
  let bx = x + 8 * s; let bars = '';
  for (const [w, c] of r.text) { bars += rect(bx, ry + 4 * s, w * s, 2 * s, t[c]); bx += (w + 2) * s; }
  const body = rect(x, ry, 6 * s, 6 * s, r.held ? t.dim : t.hair) + rect(x + 8 * s, ry, r.name * s, 2 * s, r.held ? t.ink2 : t.dim) + bars;
  return body + (r.held ? corners(x - 4 * s, ry - 3 * s, (CHAT_W + 8) * s, 12 * s, s, t.ink) : '');
}).join('');

// ---- the kit ------------------------------------------------------------------------------------------------
const files = {};
const put = (name, content) => { files[name] = content; };

// The mark on its own, no ground: ink for a light ground, ink for a dark ground.
put('svg/mark-on-light.svg', svg(16, 16, mark(0, 0, 1, L.ink), 'Niro'));
put('svg/mark-on-dark.svg', svg(16, 16, mark(0, 0, 1, D.ink), 'Niro'));
// The favicon: the ground filled, following the reader's theme.
put('svg/favicon.svg', svg(16, 16, '<rect width="16" height="16" class="g"/>' + '<path class="m" d="' + runs(MARK, 0, 0, 1) + '"/>', 'Niro',
  '.g{fill:' + L.page + '}.m{fill:' + L.ink + '}@media (prefers-color-scheme:dark){.g{fill:' + D.page + '}.m{fill:' + D.ink + '}}'));
// The app icon and bot avatar: the soft dark ground, the mark at 20 px a unit, well inside Discord's circle.
const tile = (size, unit, ground, ink) => svg(size, size, rect(0, 0, size, size, ground) + mark((size - 16 * unit) / 2, (size - 16 * unit) / 2, unit, ink), 'Niro');
put('svg/app-icon.svg', tile(512, 20, D.page, D.ink));
// The development application: the same mark, inverted, so it is never taken for the real bot.
put('svg/dev-app-icon.svg', tile(512, 20, D.ink, D.page));

// The Discord profile banner, 680 by 240 (exported at 2x). Discord puts the avatar over the lower left corner.
const banner = (dev) => {
  const t = D;
  let b = rect(0, 0, 680, 240, t.page) + mark(28, 32, 4, t.ink) + text('Niro', 112, 36, 8, t.ink);
  if (dev) b += frame(312, 52, 70, 34, t.ink2) + text('DEV', 312 + 18, 62, 2, t.ink2);
  // Below y 150 on the left is Discord's avatar: nothing goes there.
  b += text(dev ? 'Development bot.' : 'Keeps a server in order.', 32, 116, 3, t.ink2);
  b += chat(t, 488, 58, 2);
  return svg(680, 240, b, dev ? 'Niro Dev. Development bot.' : 'Niro. Keeps a server in order.');
};
put('svg/discord-banner.svg', banner(false));
put('svg/dev-discord-banner.svg', banner(true));

// The GitHub social preview, 1280 by 640.
{
  const t = D;
  const b = rect(0, 0, 1280, 640, t.page)
    + mark(72, 88, 8, t.ink) + text('Niro', 240, 96, 16, t.ink)
    + text('Keeps a server in order.', 80, 288, 4, t.ink) + text('Moderation and security for Discord.', 80, 344, 3, t.ink2)
    + chat(t, 860, 168, 4)
    + rect(80, 520, 1120, 2, t.hair) + text('niro.nithron.com', 80, 552, 3, t.dim) + text('by Nithron', 1200 - textWidth('by Nithron', 3), 552, 3, t.dim);
  put('svg/social-preview.svg', svg(1280, 640, b, 'Niro. Keeps a server in order. Moderation and security for Discord. niro.nithron.com, by Nithron.'));
}

// The seven application emoji, on the 16 grid, one grey for every Discord theme.
const EMOJI = { niro: null, shield: 'shield', lock: 'lock', ok: 'ok', warn: 'warning', error: 'failed', info: 'notice' };
for (const [name, ic] of Object.entries(EMOJI)) put('svg/emoji/' + name + '.svg', svg(16, 16, ic ? icon(ic, 0, 0, 1, EMOJI_INK) : mark(0, 0, 1, EMOJI_INK), name));

// ---- README pictures: 846 units wide, one unit a pixel at GitHub's README width ---------------------------------
const W = 846;
const windowChrome = (t, w, h, title) => rect(0, 0, w, h, t.plate) + rect(0, 0, w, 26, t.bar) + frame(0, 0, w, h, t.hair) + rect(0, 26, w, 2, t.hair)
  + [0, 1, 2].map((i) => rect(12 + i * 12, 10, 6, 6, t.dim)).join('') + text(title, Math.round((w - textWidth(title, 2)) / 4) * 2, 6, 2, t.ink2);
const typed = (t, str, x, y, s, fill, delay, id) => {
  const w = str.length * ADVANCE * s, h = 9 * s, dur = (str.length * 0.045).toFixed(2);
  return {
    body: text(str, x, y, s, fill) + '<rect class="c' + id + '" x="' + x + '" y="' + (y - s) + '" width="' + w + '" height="' + (h + 2 * s) + '" fill="' + t.plate + '"/><rect class="k' + id + '" x="' + x + '" y="' + y + '" width="' + 4 * s + '" height="' + 7 * s + '" fill="' + t.ink + '"/>',
    css: '.c' + id + '{transform-box:fill-box;transform-origin:right;animation:c' + id + ' ' + dur + 's steps(' + str.length + ') ' + delay + 's both}.k' + id + '{animation:k' + id + ' ' + dur + 's steps(' + str.length + ') ' + delay + 's both,b 1.1s steps(2) infinite}@keyframes c' + id + '{from{transform:scaleX(1)}to{transform:scaleX(0)}}@keyframes k' + id + '{from{transform:translateX(0)}to{transform:translateX(' + w + 'px)}}',
  };
};
const BASE_CSS = '@keyframes b{50%{opacity:0}}@media (prefers-reduced-motion:reduce){[class^=c]{display:none}[class^=k]{animation:none!important;display:none}}';

const HERO_LINES = [['guardian', 'hacked accounts, scam links, raids', 'cyan'], ['modules', 'moderation, logs, roles, tickets', 'blue'], ['hosted', 'niro.nithron.com', 'violet']];
const hero = (t) => {
  const h = 286;
  const line = typed(t, 'Keeps a Discord server in order.', 26, 152, 2, t.ink, 0.6, 1);
  const body = windowChrome(t, W, h, 'niro')
    + text('$', 26, 46, 2, t.dim) + text('niro status', 50, 46, 2, t.ink2)
    + mark(22, 74, 4, t.ink) + text('Niro', 104, 78, 8, t.ink)
    + line.body
    + HERO_LINES.map(([k, v, c], i) => text(k, 26, 190 + i * 22, 2, t[c]) + text(v, 26 + 10 * ADVANCE * 2, 190 + i * 22, 2, t.ink2)).join('')
    + chat(t, 640, 70, 2)
    + rect(2, h - 28, W - 4, 26, t.bar) + rect(0, h - 30, W, 2, t.hair)
    + icon('shield', 14, h - 23, 1, t.ink2) + text('alpha, on the way to v0.1.0', 38, h - 22, 2, t.ink2)
    + text('by Nithron', W - 26 - textWidth('by Nithron', 2), h - 22, 2, t.ink2);
  return svg(W, h, body, 'A terminal window. Niro: keeps a Discord server in order. Guardian catches hacked accounts, scam links and raids. Modules: moderation, logs, roles, tickets. Hosted at niro.nithron.com. Alpha, on the way to v0.1.0, by Nithron.', BASE_CSS + line.css);
};
const header = (t, words) => { const w = textWidth(words, 3); return svg(W, 40, text('>', 0, 8, 3, t.dim) + text(words, 30, 8, 3, t.ink) + rect(30 + w + 18, 18, W - (30 + w + 18), 2, t.hair), words); };
const CARD_W = 410;
const card = (t, c) => svg(CARD_W, 132, rect(0, 0, CARD_W, 132, t.plate) + frame(0, 0, CARD_W, 132, t.hair) + icon(c.icon, 22, 20, 2, t.ink) + text(c.name, 68, 22, 4, t.ink)
  + c.lines.map((l, i) => text(l, 22, 74 + i * 22, 2, t.ink2)).join(''), c.name + ': ' + c.lines.join(' '));
export const HEADERS = ['what it does', 'add niro', 'run your own', 'documentation', 'security', 'licence'];
export const CARDS = [
  { id: 'moderation', icon: 'moderation', name: 'Moderation', lines: ['warn, timeout, kick, ban,', 'purge; every action a case'] },
  { id: 'guardian', icon: 'shield', name: 'Guardian', lines: ['hacked accounts, scam links,', 'raids, a verification gate'] },
  { id: 'automod', icon: 'filter', name: 'AutoMod', lines: ["Discord's own rules,", 'managed from the dashboard'] },
  { id: 'logging', icon: 'history', name: 'Logging', lines: ['edits, deletions, joins,', 'each category its channel'] },
  { id: 'roles', icon: 'role', name: 'Role panels', lines: ['roles members pick', 'from one message'] },
  { id: 'welcome', icon: 'welcome', name: 'Welcome', lines: ['welcome and goodbye,', 'roles given on join'] },
  { id: 'tickets', icon: 'ticket', name: 'Tickets', lines: ['private threads,', 'transcripts that expire'] },
  { id: 'dashboard', icon: 'app', name: 'Dashboard', lines: ['log in with Discord;', 'settings apply live'] },
];
for (const [theme, t] of Object.entries(THEMES)) {
  put('readme/hero-' + theme + '.svg', hero(t));
  for (const h of HEADERS) put('readme/h-' + h.replace(/[^a-z0-9]+/g, '-') + '-' + theme + '.svg', header(t, h));
  for (const c of CARDS) put('readme/card-' + c.id + '-' + theme + '.svg', card(t, c));
}

// ---- write and render ---------------------------------------------------------------------------------------
for (const d of ['svg', 'readme', 'png']) rmSync(join(here, d), { recursive: true, force: true });
for (const [name, content] of Object.entries(files)) { mkdirSync(dirname(join(here, name)), { recursive: true }); writeFileSync(join(here, name), content); }

const png = (src, w, h, out) => { mkdirSync(dirname(join(here, out)), { recursive: true }); execFileSync('rsvg-convert', ['-w', String(w), '-h', String(h), join(here, src), '-o', join(here, out)]); };
for (const n of [16, 32, 48, 64, 128, 256, 512]) for (const g of ['light', 'dark']) png('svg/mark-on-' + g + '.svg', n, n, 'png/mark/mark-on-' + g + '-' + n + '.png');
png('svg/app-icon.svg', 1024, 1024, 'png/discord/app-icon-1024.png');
png('svg/app-icon.svg', 512, 512, 'png/discord/avatar-512.png');
png('svg/dev-app-icon.svg', 1024, 1024, 'png/discord/dev-app-icon-1024.png');
png('svg/discord-banner.svg', 1360, 480, 'png/discord/profile-banner-1360x480.png');
png('svg/discord-banner.svg', 680, 240, 'png/discord/profile-banner-680x240.png');
png('svg/dev-discord-banner.svg', 1360, 480, 'png/discord/dev-profile-banner-1360x480.png');
for (const name of Object.keys(EMOJI)) png('svg/emoji/' + name + '.svg', 128, 128, 'png/discord/emojis/' + name + '.png');
png('svg/social-preview.svg', 1280, 640, 'png/social/github-social-preview.png');
// Web: the tab icon follows the theme (favicon.svg); the bitmaps are on the dark ground, whole pixels at each size.
const webTile = (size, unit, out) => { const f = 'png/web/.tile-' + size + '.svg'; mkdirSync(join(here, 'png/web'), { recursive: true }); writeFileSync(join(here, f), tile(size, unit, D.page, D.ink)); png(f, size, size, out); rmSync(join(here, f)); };
webTile(16, 1, 'png/web/icon-16.png'); webTile(32, 2, 'png/web/icon-32.png'); webTile(48, 3, 'png/web/icon-48.png');
webTile(180, 10, 'png/web/apple-touch-icon.png'); webTile(192, 12, 'png/web/icon-192.png'); webTile(512, 32, 'png/web/icon-512.png');
execFileSync('magick', [join(here, 'png/web/icon-16.png'), join(here, 'png/web/icon-32.png'), join(here, 'png/web/icon-48.png'), join(here, 'png/web/favicon.ico')]);
copyFileSync(join(here, 'svg/favicon.svg'), join(here, 'png/web/favicon.svg'));
for (const f of ['favicon.svg', 'favicon.ico', 'apple-touch-icon.png', 'icon-192.png', 'icon-512.png']) copyFileSync(join(here, 'png/web', f), join(root, 'web/public', f));
console.log(Object.keys(files).length + ' SVG sources and the PNGs written');
