# Nithro — Discovery & Recommendations

*Prepared 2026-11-22 for the project manager. No code has been written yet; this report is the input for choosing a direction and building the plan.*

---

## TL;DR

- The Discord bot market is **crowded at the generalist end** (MEE6, Dyno, Carl-bot) and **fragmented at the specialist end** (Wick for anti-raid, Ticket Tool, Arcane for leveling, YAGPDB for scripting). Most servers run 4–6 bots to cover their basics.
- The openings are **trust and cost**, not missing features: aggressive paywalls (MEE6), steep configuration (Carl-bot), closed source, privacy worries after Discord's 2026 age-verification rollout, and a wave of **compromised accounts posting scam links**, which current bots handle poorly.
- **Recommended direction:** a *security-first, open-source, all-in-one community bot* with a polished web dashboard, plus a privacy-respecting **AI layer** powered by **nAI** (bring your own model, local by default). It fits the Nithron brand (local-first, privacy-first, open source, self-hostable), and no major generalist bot occupies that position today.
- **Recommended stack:** a TypeScript monorepo with **discord.js**, **PostgreSQL** (+ pgvector), **Valkey/Redis**, and a **Next.js** dashboard, shipped via Docker Compose so it is self-hostable (and runs on NithronOS). Go is a close second; the trade-offs are in §5.

---

## 1. Market landscape

| Bot | Known for | Weak spots (what users complain about) |
|---|---|---|
| **MEE6** | Biggest name; leveling, welcome, music | Features that used to be free moved behind Premium (~$11.99/mo). In early 2026 there were reports of paid add-ons (e.g. ~$90 to rename the bot). Many negative reviews about paywalls. |
| **Dyno** | Automod, moderation | Dated UX; premium per server |
| **Carl-bot** | Reaction roles, logging, custom commands | Steep learning curve |
| **Wick** | Anti-raid / anti-nuke | Narrow focus; complex |
| **Sapphire, Onyx** | Free moderation | Smaller ecosystems |
| **YAGPDB** | Scripting for power users | Hard to use; niche |
| **Arcane** | Leveling incl. voice XP | Single-purpose |
| **Ticket Tool** | Support tickets | Single-purpose |
| **PeakBot, Bond, MurFFy** (new 2025–26) | "Replace 4+ bots with one" | Young; closed; mostly marketing claims |
| **AI bots** (Tenka, Modcord, GPTDiscord…) | KB Q&A, summaries, context-aware moderation | AI usually paywalled; data sent to third-party LLMs; generic answers when there's no KB |

**Takeaways**
1. "All-in-one" is already a pitch other bots make, so it can't be the main differentiator. What would set Niro apart is being **open, trustworthy, generous, and secure**.
2. AI features are spreading, but they are usually **cloud-only, opaque and paywalled**. A bot that lets a server bring its own model or run one locally would stand out.
3. Security is still **reactive**. Compromised accounts post "MrBeast giveaway" or crypto links across every channel within seconds, and RaidProtect reported *tens of thousands* of compromised accounts in 2026.

## 2. Discord platform realities (they shape scope)

- **Bot verification at 100 servers** requires identity verification of the owner. **Privileged intents** (Message Content, Server Members, Presence) then need approval with a justification. *Any feature that reads message text (automod beyond Discord's native AutoMod, leveling, AI) depends on that approval.* Niro should be designed for it: use Discord's native **AutoMod API** where possible, and have a written use-case justification ready.
- **Components V2** (containers, sections, media galleries, separators) lets bots build rich, app-like messages. It is supported in discord.js, discord.py, JDA and Discord.Net, and it is a real UX advantage over bots that still use legacy embeds.
- **User-installable apps** are GA, so commands can work in DMs or any server where the user is a member. This fits personal utilities and personal AI.
- **New endpoints (Oct 2026)** return the computed permissions of a member or role, and the audit log now covers channel and role position changes. Both make **permission audits and anti-nuke checks** much easier to build.
- **Monetization:** Discord's Premium Apps/SKUs (in-client subscriptions, one-time purchases) or Nithron's own Stripe billing, to be picked later.
- **Sharding** is mandatory at 2,500 guilds. Every library under consideration handles it, but clustering needs a plan if Niro grows large.
- **Platform risk:** the 2026 age-verification rollout caused a visible (if probably small) move to **Stoat** (formerly Revolt), **Fluxer** and TeamSpeak. Fluxer's API is deliberately Discord-like. Keeping Niro's core **platform-agnostic** is a cheap hedge.

## 3. Product directions (pick one, or a combination)

### A. Guardian: security-first protection ⭐ strongest edge
- **Compromised-account detection:** the same link or message posted across several channels within seconds, a new link domain from an old account, or a message that matches known scam templates. The bot quarantines the account (timeout plus role strip), deletes the copies, and DMs the user recovery steps.
- **Anti-raid:** join-velocity detection, account-age gates, lockdown mode, verification (button or captcha).
- **Anti-nuke:** rate-limit destructive admin actions (mass bans, channel or role deletes), auto-revert from the audit log, and alert the owner.
- **Permission audit:** flags dangerous permissions on roles, bots and webhooks, using the new computed-permission endpoints.
- **Scam-domain and phishing feed**, shared across servers that opt in.

### B. Community toolkit: replace the bot stack
Moderation (cases, warns, timeouts, appeals), automod, rich logging with before/after diffs, button and select-menu roles, welcome/goodbye, leveling (text and voice), tickets with transcripts, forms and applications, scheduled and recurring messages, a Components V2 message builder, and a free custom bot name/avatar (MEE6 charges for this).

### C. AI assistant (powered by nAI)
- **Server knowledge base:** answers questions from rules, pinned messages, FAQ channels and uploaded docs, **with citations**. It can also auto-answer support-forum posts.
- **Catch-up summaries** ("what did I miss in #general?") and thread summaries.
- **Context-aware moderation:** an LLM second opinion on borderline messages. It flags messages for moderators and never acts alone.
- **Translation** across the languages the server uses.
- **Bring your own model:** Ollama locally, any OpenAI-compatible server, or a cloud API. Privacy is the default.

### D. Nithron ecosystem bridge
NithronOS alerts in Discord (disk SMART, pool health, backup results, updates), nAI search from Discord, and an official bot for the Nithron community server. The market is niche, but it supports the brand and lets Nithron test Nithro on its own products first.

### E. Cross-platform core (architecture choice, not a product)
Discord adapter first, with the domain logic kept free of discord.js types. Fluxer and Stoat adapters could come later.

**Recommendation:** **A + B as the core product, C as the premium differentiator, D as a free integration, built the E way.**
Positioning: *"The open-source, privacy-first guardian for your community. One bot instead of five, with free core features and AI that runs on your own hardware."*

### Suggested MVP (v0.1) scope
1. Moderation and cases, plus native AutoMod management
2. Compromised-account and scam-link protection, and anti-raid basics
3. Logging with diffs
4. Button roles, welcome
5. Tickets
6. Dashboard: Discord login, server picker, configuration for all of the above, cases view

Later: leveling, forms, anti-nuke auto-revert, the AI layer, NithronOS bridge, premium.

## 4. Business model options

| Model | Notes |
|---|---|
| **Generous free hosted tier + paid Premium** (recommended) | Premium covers features that cost money to run: AI usage, long log retention, higher limits, priority support. The core protection features stay free. |
| **Open source + self-host for free** | Matches NithronOS/nAI. A license like the Nithron Community License keeps commercial rights. Builds trust and contributors. |
| Discord Premium Apps vs Stripe | Discord SKUs mean less friction inside the client. Stripe keeps more control and can be used outside Discord. Can offer both. |

## 5. Technology recommendations

### Language and library

| Option | Pros | Cons | Fit |
|---|---|---|---|
| **TypeScript + discord.js** ⭐ | Largest ecosystem; new Discord features (Components V2 etc.) land first; one language for bot, API and dashboard; shared types; Flow and nithron-site already use TS/pnpm/turbo | Higher memory per shard than Go/Rust (fine up to tens of thousands of guilds; hybrid sharding later) | **Recommended** |
| **Go + disgo** | Low memory, simple deploys, matches the NithronOS/Flow backends | Smaller Discord community; new API features arrive later (discordgo lags most, so disgo is the better choice); dashboard still needs TS | Strong alternative |
| **Python + discord.py** | Same language as nAI; easy AI work | Slower; AI can be a separate service anyway | Not recommended for the core |
| **Rust + twilight/serenity** | Best performance at huge scale | Slowest to develop; overkill before 100k guilds | Not now |

### Data

| Need | Choice | Why |
|---|---|---|
| Primary store | **PostgreSQL 17+** | Relational data (guilds, cases, tickets, levels, audit); JSONB for flexible per-module config |
| Cache, rate limits, cooldowns, queues, pub/sub | **Valkey** (open-source Redis fork) | Dashboard → bot live config push, XP cooldowns, BullMQ job queues, raid counters |
| AI embeddings | **pgvector** inside Postgres, or delegate to **nAI** | No separate vector DB to run |
| ORM / migrations | **Drizzle** | Type-safe, SQL-first, light |
| Analytics (later, only if needed) | TimescaleDB or ClickHouse | Message/member stats at scale |

Not recommended: MongoDB. It's common in hobby bots, but Niro's data is relational, and Postgres with JSONB covers the flexible parts.

### Admin panel / dashboard
- **Next.js (App Router) + Tailwind + shadcn/ui**, with **Discord OAuth2** login (Auth.js or Better Auth). Access is limited to users with *Manage Server* in that guild.
- **API service:** **Hono** (or Fastify), shared by the dashboard and a future public API.
- **Real-time:** the dashboard writes config to Postgres, then publishes over Valkey pub/sub, and the bot applies it immediately without a restart.
- Includes a live Components V2 message preview, case and ticket browsers, and an audit trail of every dashboard change.

### Proposed repo layout (for the plan, nothing created yet)
```
apps/bot         discord.js gateway + interactions
apps/api         Hono REST API
apps/dashboard   Next.js admin panel
packages/core    platform-agnostic domain logic (modules: moderation, guardian, tickets…)
packages/db      Drizzle schema + migrations
packages/shared  types, zod schemas, config
infra/           docker-compose (postgres, valkey), deployment
docs/
```
Ops: Docker Compose (self-host and NithronOS app catalog), GitHub Actions CI, Vitest, Sentry or OpenTelemetry, Prometheus metrics.

## 6. Risks

| Risk | Mitigation |
|---|---|
| Privileged-intent approval denied | Use the native AutoMod API; justify Message Content clearly; degrade gracefully when intents are missing |
| "Yet another all-in-one" fatigue | Lead with Guardian (security) and open source; don't lead with the feature count |
| LLM costs | AI is premium or bring-your-own-model; local nAI by default; per-guild quotas |
| Moderation false positives | Quarantine rather than ban; one-click mod review; tunable thresholds |
| Discord platform churn | Platform-agnostic core (E) |

## 7. Decisions needed from the PM

1. **Direction:** A / B / C / D, or the recommended A+B core, C premium, D integration.
2. **Language:** TypeScript (recommended) or Go.
3. **Open source and license:** public repo under the Nithron Community License like NithronOS/nAI, or closed? (The GitHub repo was created **private** until this is decided.)
4. **Hosting model:** hosted SaaS, self-host, or both.
5. **MVP scope:** confirm or trim the v0.1 list in §3.
6. **Monetization:** Discord Premium Apps, Stripe, both, or not yet.

---

### Sources
- [MEE6 vs Dyno vs Carl-bot 2026 (PeakBot)](https://peakbot.pro/blog/mee6-vs-dyno-vs-carl-bot-2026) · [Best Carl-bot alternatives 2026](https://peakbot.pro/blog/best-carl-bot-alternatives-2026) · [MEE6 alternatives 2026 (Bond)](https://www.bondbot.gg/blog/mee6-alternatives-2026) · [Best Discord bots 2026 (MurFFy)](https://murffy.xyz/en/blog/best-discord-bots) · [Collony: MEE6 alternatives](https://www.collony.ai/blog/best-mee6-alternative). *Vendor-written; treat rankings with skepticism.*
- [MEE6 Trustpilot reviews](https://www.trustpilot.com/review/mee6.xyz?page=1) · [Why people hate MEE6 (Medium)](https://medium.com/netcord/why-people-hate-mee6-better-alternatives-733fae94217e) · [Jan 2026 MEE6 add-on pricing write-up](https://lilys.ai/en/notes/telegram-20260128/1000-dollar-discord-bot) *(secondhand)*
- [Discord developer change log](https://docs.discord.com/developers/change-log) · [Application resource / install contexts](https://docs.discord.com/developers/resources/application) · [Components reference](https://docs.discord.com/developers/components/reference) · [discord.js guide: Components V2](https://discordjs.guide/popular-topics/components-v2) · [Discord Developer Portal 2026 changes](https://openclawlaunch.com/blog/discord-developer-portal-2026-changes) · [New Discord features Nov 2026](https://betterantispam.com/en/blog/new-discord-features)
- [Bitdefender: stolen accounts spreading fake MrBeast giveaways](https://www.bitdefender.com/en-us/blog/hotforsecurity/hackers-are-using-stolen-discord-accounts-to-spread-fake-mrbeast-giveaways) · [CyberDaily: wave of account takeovers](https://www.cyberdaily.au/security/13878-bro-a-wave-of-discord-account-takeovers-is-spreading-a-mrbeast-crypto-scam) · [Kaspersky: Discord scams](https://www.kaspersky.com/resource-center/threats/discord-scams)
- [TechRadar: users flocking to Stoat](https://www.techradar.com/computing/social-media/so-many-discord-users-are-flocking-to-this-alternative-platform-its-making-stoat-crash) · [Windows Central: alternative searches +10,000%](https://www.windowscentral.com/software-apps/discord-alternative-search-10000-percent-stoat) · [Fluxer quickstart (Discord-like API)](https://docs.fluxer.app/quickstart) · [Dev.to: 9 years on Discord, next best is Fluxer](https://dev.to/nevulo/ive-spent-9-years-on-discord-the-next-best-option-is-fluxer-15gh)
- [Best Discord bot libraries 2026](https://space-node.net/blog/best-discord-bot-libraries-2026) · [discord.js vs discord.py vs Serenity](https://discordcraft.com/blog/discordjs-vs-discordpy-vs-serenity-best-bot-framework-in-2026) · [Highly available Discord bots 2026](https://cloud.a-line.be/blog/highly-available-discord-bots/) · [Large bots and their libraries (gist)](https://gist.github.com/advaith1/451dcbca2d7c3503d4f48d63eb918cb0)
- [eesel: Discord AI bots 2026](https://eesel.ai/blog/discord-ai) · [Quickchat: best AI Discord bots](https://quickchat.ai/post/best-ai-discord-bots) · [Botpress: Discord AI chatbot](https://botpress.com/blog/discord-ai-chatbot)
