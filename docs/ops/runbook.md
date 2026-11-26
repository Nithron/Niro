# Hosted Niro runbook

How the hosted Niro (`niro.nithron.com`) runs and what to do when something happens. The setup is [`deploy/hosted/compose.yml`](../../deploy/hosted/compose.yml) on one VPS (decision 28).

## Layout

| Service | What it does | Reachable from |
|---|---|---|
| `caddy` | HTTPS for `niro.nithron.com`, proxies to `api` | the internet (80, 443) |
| `api` | Dashboard and API (`niro api`) | Caddy |
| `bot` | The Discord bot, every shard (`niro bot`). **One replica only** | not exposed |
| `migrate` | Runs database migrations, then exits. `bot` and `api` wait for it | not exposed |
| `postgres`, `valkey` | Data and caches | the compose network |
| `backup`, `offsite` | Nightly dump at 03:00 UTC, copied off-site with restic at 04:00 | not exposed |
| `prometheus`, `alertmanager`, `node-exporter` | Metrics and alerts to the ops channel in the support server | the compose network |
| `grafana` | Dashboards | `127.0.0.1:3000` on the server (SSH tunnel) |

Every command below runs on the server, in the repository checkout, with:

```sh
alias dc='docker compose -f deploy/hosted/compose.yml'
```

## First setup

**What you do:**

- **The VPS:** Debian or Ubuntu, 4 GB RAM or more, with SSH key access.
- **DNS:** `A` and `AAAA` records for `niro.nithron.com` pointing at the VPS.
- **Production application (Developer Portal, Niro):**
  - Redirect `https://niro.nithron.com/auth/callback`.
  - Both privileged intents on (Server Members and Message Content; this needs verification past 100 servers).
- **Alerts webhook:** a private `#ops-alerts` channel in the support server, with a webhook.
- **Off-site storage:** a restic repository, for example a storage box (SFTP) or S3-compatible storage.
- **Backup key:** an age key pair made **off** the server (`age-keygen`). Keep the private key in the team's password manager.

**On the server:**

1. Install Docker Engine and the Compose plugin from Docker's apt repository, then turn on automatic security updates (`unattended-upgrades`).
2. Firewall: allow SSH, 80/tcp, 443/tcp and 443/udp, and nothing else. In `sshd_config`, turn off password login (once key login works) and root login.
3. Get the code at the release tag:
   ```sh
   git clone https://github.com/Nithron/Niro.git && cd Niro && git checkout v0.1.0
   ```
4. Make the settings files, all `chmod 600`:
   - `deploy/hosted/.env`, from `.env.example`. Put the age **public** key in `BACKUP_AGE_RECIPIENT`.
   - `deploy/hosted/offsite.env`, from `offsite.env.example`.
   - `deploy/hosted/secrets/discord_webhook_url`: the webhook URL, on one line.
5. Start:
   ```sh
   dc up -d --build
   ```
   Check that `dc ps` shows `migrate` exited 0 and everything else running.
6. Copy off-site once by hand (`dc exec offsite sh /offsite/offsite.sh now`) and check `dc exec offsite restic snapshots`.
7. Set up an external uptime check on `https://niro.nithron.com/` (any uptime service, every minute) that alerts by email. It covers the case where the whole server is down and can't alert by itself.

## Deploy a release

1. Read the changelog. A new migration is applied by the `migrate` step.
2. Take a backup:
   ```sh
   dc exec backup sh /backup/backup.sh now
   ```
3. Update:
   ```sh
   git fetch --tags && git checkout vX.Y.Z
   ```
   Then set `NIRO_VERSION=vX.Y.Z` in `deploy/hosted/.env`.
4. Roll out:
   ```sh
   dc up -d --build
   ```
   `migrate` runs first, then `bot` and `api` restart. The bot reconnects with a fresh session, which takes a few seconds per shard. Gateway events during the restart are missed.
5. Watch for 15 minutes: `dc logs -f bot api`, the Grafana dashboard, and `#ops-alerts`.

## Roll back

- **The release had no new migration:** check out the previous tag, set `NIRO_VERSION` back, and run `dc up -d --build`.
- **It had a migration:** the old code may not work with the new schema.
  1. Stop `bot` and `api`.
  2. Restore the backup from step 2 above ([backup and restore](backup-restore.md)).
  3. Deploy the previous tag.

  Data written since the backup is lost. So keep migrations additive (new tables and columns), so that most rollbacks are the first case.

## Rotate a secret

| Secret | How |
|---|---|
| Bot token | Developer Portal → Bot → Reset Token. Update `.env`, then `dc up -d bot` |
| OAuth client secret | Developer Portal → OAuth2 → Reset Secret. Update `.env`, then `dc up -d api`. Dashboard users sign in again |
| Postgres password | `dc exec postgres psql -U niro -c "ALTER USER niro PASSWORD '…'"`. Update `.env`, then `dc up -d` |
| Valkey password | Update `.env`, then `dc up -d valkey bot api`. Everyone is signed out of the dashboard |
| Grafana admin | Update `.env`, then `dc exec grafana grafana cli admin reset-admin-password '…'` |
| restic password | `dc exec offsite restic key add`, then `restic key remove <old id>`. Update `offsite.env` |
| Backup age key | Make a new pair and put the new public key in `.env`. **Keep the old private key for 28 days**, until the last dump encrypted with it is gone |
| Alerts webhook | Delete and recreate it in Discord, update `secrets/discord_webhook_url`, then `dc restart alertmanager` |

If a secret leaked, rotate it first, then read the logs for anything done with it.

## Alerts

| Alert | Meaning | First steps |
|---|---|---|
| `NiroDown` | Prometheus can't scrape `bot` or `api` | `dc ps`, `dc logs --tail 100 <service>`, `dc up -d` |
| `NiroShardDown` | A shard has been disconnected for 5 minutes | Check [discordstatus.com](https://discordstatus.com), then `dc logs bot | grep -i gateway`. If Discord is fine, `dc restart bot` |
| `NiroGatewayReconnecting` | Repeated full reconnects | Network trouble or a Discord incident. See [baselines](baselines.md#shard-resume) for normal behaviour |
| `NiroGatewayLatencyHigh` | Heartbeat above 1 s | Host CPU or network. Check node metrics in Grafana |
| `NiroDiscordRateLimited` | Discord answers 429 often | Grafana → "Discord 429s by route" shows the feature. A bug or a huge server |
| `NiroDiscordErrors` | Discord answers 5xx | Usually Discord's side. Watch discordstatus.com |
| `NiroAPIErrors` | More than 5% of API requests fail | `dc logs api | grep '"level":"ERROR"'` |
| `NiroRateLimiterDown` | The API can't reach Valkey for rate limits | `dc ps valkey`, `dc logs valkey` |
| `NiroDBPoolExhausted`, `NiroDBPoolWaiting` | Requests wait for database connections | Look for slow queries (`pg_stat_activity`). Raise `pool_max_conns` in the URL if the load is real |
| `NiroMemoryHigh` | More than 1.5 GB in one process | Compare with [baselines](baselines.md). Take a heap profile before restarting |
| `NiroJobStale`, `NiroJobFailing` | A background job stopped succeeding | `dc logs bot | grep <job name>`. Scheduler: temp-bans aren't lifted. Retention: data isn't deleted on time (privacy) |
| `NiroBackupMissing`, `NiroBackupNeverRan` | No good dump in 26 hours | `dc logs backup`, then `dc exec backup sh /backup/backup.sh now` |
| `NiroOffsiteCopyMissing` | Dumps not copied off-site in 26 hours | `dc logs offsite`, `dc exec offsite restic snapshots` |
| `HostDiskLow`, `HostMemoryLow` | The VPS is running out | `docker system df`, `docker image prune`, old logs. Then resize the VPS |

## Regular checks

- **Monthly:** run the restore drill ([backup and restore](backup-restore.md#test-restores)) and note the date and result below.
- **Monthly:** `git pull` the base images by rebuilding (`dc build --pull && dc up -d`). Reboot the VPS if `/var/run/reboot-required` exists.
- **When a release changes the alert rules:** `dc restart prometheus alertmanager`.

### Log

| Date | What | Result |
|---|---|---|
| | | |
