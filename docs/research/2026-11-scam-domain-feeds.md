# Scam-domain feeds (2026-11-22)

Guardian needs a list of known phishing/scam domains. Requirements: a licence that allows use in a source-available, commercially hosted bot; a plain download (no per-request lookups of user links, which would leak what members post); reasonable freshness.

| Source | Licence | Access | Notes |
|---|---|---|---|
| [nikolaischunk/discord-phishing-links](https://github.com/nikolaischunk/discord-phishing-links) | MIT | Static JSON/TXT on GitHub | ~21.9k domains, Discord-focused. Last commit March 2024, so new domains are missing |
| [FishFish](https://github.com/fishfish-gg) | Not found | API, some routes need a token | Active and Discord-focused, but no licence or terms found for the data; per-domain lookups would send user links to a third party |
| Sinking Yachts | MIT (client code only) | API | Being discontinued in favour of FishFish |

**Decision:** use the nikolaischunk list as the default feed (decision 18). Niro downloads it every 6 hours, keeps it in Postgres (`scam_domains`, one row per domain per feed) and never queries anything per message. Because the list is stale, Guardian's look-alike checks (homoglyphs, one-letter typos, brand + scam words, official domains used as a prefix) carry most of the weight for new domains. Feeds are configurable (`NIRO_SCAM_FEEDS`, https only, JSON `{"domains": []}`, a JSON array, or one domain per line); Discord's own domains are never accepted from a feed.

**Revisit:** if FishFish publishes data terms that allow redistribution, add it as a second feed.
