# Domain options for Nithro

*2026-11-22. Not decided yet; needed by M5 (production OAuth redirect, dashboard, docs).*

DNS check on 2026-11-22: `nithro.com` is **parked for sale** (nameservers at Afternic, so likely a premium price). `nithro.gg`, `.bot`, `.app`, `.dev`, `.io`, `.xyz` and `getnithro.com` have **no DNS at all**, which usually means unregistered. Confirm at a registrar before relying on that.

| Option | Example layout | Pros | Cons |
|---|---|---|---|
| **`nithro.gg`** ⭐ | `nithro.gg` (site), `app.nithro.gg` (dashboard), `docs.nithro.gg` | The Discord-bot convention (`dyno.gg`, `carl.gg`, `top.gg`); short; looks like a product | `.gg` usually costs more per year than `.com`/`.app` |
| `nithro.app` | `nithro.app`, `nithro.app/dashboard` | Clean; HTTPS is enforced for the whole TLD (HSTS preload) | Less "Discord" feel |
| `nithro.bot` | `nithro.bot` | Says exactly what it is | `.bot` has had registration restrictions in the past; check eligibility |
| **`bot.nithron.com`** (free) | `bot.nithron.com`, `bot.nithron.com/dashboard` | Nothing to buy; reuses nithron.com's Cloudflare setup; ties the bot to the studio brand | Longer; weaker standalone brand |
| `nithro.com` | – | The obvious name | For sale at a marketplace; probably expensive |

**Suggestion:** register `nithro.gg` as the product home, with `app.` for the dashboard and `docs.` for docs. Point `bot.nithron.com` at it so the studio site links cleanly. If you'd rather not buy a domain, `bot.nithron.com` alone works.
