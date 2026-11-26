# Self-hosting Niro

Run your own Niro on a Linux server with Docker. You get the same bot and dashboard as the hosted Niro, under your own Discord application.

## Before you start

- **Licence.** Niro is source-available under the Nithron Community License (NCL).
  - **Free:** individuals and non-commercial communities can self-host Niro for free.
  - **Paid:** a company or brand running Niro for its own server needs a commercial licence. See [COMMERCIAL.md](../COMMERCIAL.md) or write to licensing@nithron.com.
- **Your own name.** Your instance runs under your own Discord application, with its own name and avatar. Niro's name and brand assets are not licensed for other instances ([TRADEMARK_POLICY.md](../TRADEMARK_POLICY.md)).
- **You need:**
  - A Linux server with Docker Engine and the Compose plugin (`docker compose version`).
  - 2 GB of RAM is enough for a few hundred servers. See [baselines](ops/baselines.md) for measured numbers.
  - A domain name pointing at the server, with ports 80 and 443 open, for HTTPS.
- **You are the data controller** for your instance. Niro stores moderation cases, settings and ticket transcripts about the people in your servers. See [Data and privacy](#data-and-privacy).

### On nOS

nOS ships Docker Engine and Compose v2 for its app catalog, so Niro runs on nOS with this guide like on any Debian or Ubuntu server. Niro is not in the nOS app catalog yet. One difference is HTTPS:

- nOS's own Caddy holds ports 80 and 443, so leave out `--profile tls` in step 4.
- nOS's app snippets (`/etc/caddy/Caddyfile.d/app-*.caddy`) serve apps under `/apps/<id>/` of the nOS site. Niro's dashboard needs the root of its own domain (Discord login and cookies), so it can't go there.
- Instead, add a site for Niro's domain to `/etc/caddy/Caddyfile`, outside the `:443 { … }` block. nOS turns Caddy's automatic certificates off, so give it a certificate for that domain (for example from certbot with a DNS challenge):
  ```caddy
  niro.example.com {
      tls /etc/ssl/niro/fullchain.pem /etc/ssl/niro/privkey.pem
      reverse_proxy 127.0.0.1:8080
  }
  ```
  Then `sudo caddy validate --config /etc/caddy/Caddyfile` and `sudo systemctl reload caddy`. Changing nOS's remote access settings rewrites that file, so check the site is still there afterwards.
- **Firewall:** nOS allows 80 and 443 from the local network only. A dashboard used from the internet needs them open publicly (nOS: Remote access, Public HTTPS). The bot itself needs no open ports: it only connects out to Discord.

Niro's compose network (`172.31.250.0/24`) doesn't overlap nOS's Docker address pool (`172.28.0.0/16`). Requests from the host's Caddy count as trusted, so rate limits see each visitor's own address.

## 1. Create a Discord application

1. Open the [Discord Developer Portal](https://discord.com/developers/applications) and click **New Application**.
2. **Bot** page:
   - Click **Reset Token** and keep the token for step 3. Never share it.
   - Turn on **Server Members Intent** and **Message Content Intent**. Guardian, the server log and welcome messages need them. A bot in fewer than 100 servers can turn them on without review.
   - Turn **Public Bot** off if only you should be able to add it.
3. **OAuth2** page:
   - Note the **Client ID** and reset the **Client Secret**.
   - Add the redirect `https://<your domain>/auth/callback`.

## 2. Get the code

```sh
git clone https://github.com/Nithron/Niro.git
cd Niro
git checkout v0.1.0          # the latest release tag
```

Niro is built from source on your server. There is no prebuilt image for v0.1.0.

## 3. Configure

```sh
cp deploy/.env.example deploy/.env
chmod 600 deploy/.env
```

Fill in `deploy/.env`:

- `NIRO_DISCORD_TOKEN`, `NIRO_DISCORD_CLIENT_ID`, `NIRO_DISCORD_CLIENT_SECRET`: from step 1.
- `NIRO_PUBLIC_URL` (for example `https://niro.example.com`) and `NIRO_DOMAIN` (`niro.example.com`).
- `POSTGRES_PASSWORD` and `VALKEY_PASSWORD`: long random values made of letters and digits. `openssl rand -hex 32` makes one. Characters such as `@ : / ? #` would break the database URL.

The compose network uses `172.31.250.0/24`. If your server already uses that range, change the `subnet` and `NIRO_TRUSTED_PROXIES` in `deploy/compose.yml` to match.

## 4. Start

With the bundled Caddy, which gets and renews the HTTPS certificate for you:

```sh
docker compose -f deploy/compose.yml --profile tls up -d --build
```

The first build takes a few minutes. Then:

- `docker compose -f deploy/compose.yml ps` shows `niro`, `postgres`, `valkey`, `backup` and `caddy` running.
- `docker compose -f deploy/compose.yml logs niro` shows `bot online`.

**With your own reverse proxy** (nginx, Traefik…):

- Leave out `--profile tls`.
- Point the proxy at `http://127.0.0.1:8080`, and make it set `X-Forwarded-For`.
- Niro trusts that header only from the compose network, which includes requests from this host. A proxy elsewhere must be added to `NIRO_TRUSTED_PROXIES`. Otherwise every visitor shares one rate limit.

## 5. Add Niro to your server

Open `https://<your domain>`, log in with Discord, and click **Add Niro** next to your server. Then turn on the modules you want.

## Checking on it

- **Health:** `docker run --rm --network niro_niro curlimages/curl -s niro:9090/readyz`. It answers `"ready":true` when Postgres, Valkey and every gateway shard are fine.
- **Logs:** `docker compose -f deploy/compose.yml logs -f niro` (JSON).
- **Metrics:** Prometheus can scrape `niro:9090/metrics` on the compose network. Alert rules, an Alertmanager config and a Grafana dashboard are in [`deploy/monitoring/`](../deploy/monitoring/).
- **Backup alerts:** these need node_exporter to read the `backup-metrics` volume as its textfile directory (`--collector.textfile.directory`). [`deploy/hosted/compose.yml`](../deploy/hosted/compose.yml) shows the full setup.

## Updating

```sh
docker compose -f deploy/compose.yml exec backup sh /backup/backup.sh now   # a fresh backup first
git fetch --tags && git checkout v0.1.1                                      # the new tag
docker compose -f deploy/compose.yml --profile tls up -d --build
```

Database migrations run automatically when Niro starts. Read the [changelog](../CHANGELOG.md) before you update.

## Backups

The `backup` service:

- dumps the database when it first starts, then every night at 03:00 UTC, to `deploy/backups/`;
- keeps 7 daily and 3 weekly copies;
- can encrypt the dumps.

Copy that folder off the server. See [backup and restore](ops/backup-restore.md), which also covers restoring and testing a restore.

## Data and privacy

- **What is stored:**
  - Settings, moderation cases, Guardian events (evidence summaries, never message text), role and ticket panels.
  - Ticket transcripts. These are the only message text in the database: kept 90 days by default, set per server between 1 and 365 days.
- **The server log's message cache** lives in Valkey only, for 7 days.
- **When Niro leaves a server,** that server's data is deleted after 30 days. The owner can also delete it at once from the dashboard: **Delete data…** on the server's page.
- **Backups** follow the same limits: the oldest is at most 28 days old.
- Tell your members what you run, for example in your server rules. The hosted Niro's privacy policy is a reasonable template.

## Troubleshooting

- **`NIRO_PUBLIC_URL must use https in production`:** set an https URL. The dashboard's login cookie needs it.
- **Login fails with a redirect error:** the redirect in the Developer Portal must be exactly `NIRO_PUBLIC_URL` + `/auth/callback`.
- **`Message Content intent is not enabled` in the logs:** turn the intent on (step 1) and restart `niro`.
- **"Too many requests" for every user:** your reverse proxy isn't trusted. See step 4.
