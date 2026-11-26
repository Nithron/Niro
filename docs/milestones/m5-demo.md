# M5 demo checklist: Hardening and launch

What to check before M5 is accepted. Part A is what's built and can be checked now. Part B is the launch, which needs your actions (hosting, Discord, legal) and ends with the `v0.1.0` tag.

## Part A: what's built

**Setup:** the M5 branch running locally (`make run`) with Niro Dev in the test server, as in earlier demos.

### Security

1. **Tests:** CI is green on the M5 pull request, including the new jobs: Dependency audit, Fuzz parsers, Load test (one minute), Deployment config.
2. **Log out on all devices:**
   1. Open the dashboard in two browsers and log in to both.
   2. In one, open the account menu (your avatar) and choose **Log out on all devices**.
   3. Both browsers are back at the login screen.
3. **Idle timeout:** a session unused for 24 hours ends. Covered by tests; nothing to click.
4. **Headers:** in the browser's developer tools, an `/api/v1/me` response has `Cache-Control: no-store`, `Cross-Origin-Opener-Policy: same-origin` and a `Permissions-Policy`.

### Your data

5. **Delete data (owner only):** use a throwaway server that you own, with Niro in it. Don't use the test server unless you're ready to re-invite Niro.
   1. On the server's page, click **Delete data…**.
   2. The button stays disabled until you type the server's exact name.
   3. Confirm. Niro leaves the server, and the dashboard returns to your server list.
6. **Not for managers:** on a server you manage but don't own, there is no **Delete data…** card.
7. **Legal pages:** `/terms` and `/privacy` open without logging in, and are linked from the login screen and the dashboard footer. They name Nithron as the controller and HostStronger (Germany) as the host.

### Self-hosting and backups

8. **Self-host guide:** read [docs/self-host.md](../self-host.md). I ran it from a clean copy of the repository on my machine:
   - all five services came up;
   - Caddy served HTTPS and redirected HTTP;
   - the bot came online;
   - a backup was written and restored into an empty database with matching row counts.

   Running it on a fresh VPS (or on nOS) is step 13.
9. **Backups:**
   1. Run `docker compose -f deploy/compose.yml exec backup sh /backup/backup.sh now` in a running self-host stack. A dump appears in `deploy/backups/`, readable only by its owner.
   2. Stop Postgres and run it again. It reports `pg_dump failed`, and the existing dumps stay.

### Monitoring and load

10. **Monitoring:** `make deploy-check` validates the alert rules and runs their unit tests (Niro stopped, a shard down, a stale job, a 429 storm). It also validates the Alertmanager config and both compose files.
11. **Load test:** [docs/ops/baselines.md](../ops/baselines.md) has the 2-hour soak results (2,500 servers, 100 messages a second) and the shard-resume test against Niro Dev.

## Part B: launch (needs you)

12. **Hosting:** the HostStronger VPS (Ubuntu, 24 GB RAM, Falkenstein, Germany), shared with the website and Minecraft ([Shared server](../ops/runbook.md#shared-server)). Set up on 2026-11-26 except what needs you: the `niro` DNS record at Cloudflare, one `import` line in the website's Caddyfile, the production Discord credentials and the alerts webhook.
13. **nOS:** answered from the nOS docs. nOS has Docker Engine and Compose v2, so the self-host guide works there; only the dashboard's HTTPS differs, because nOS's Caddy holds ports 80 and 443 ([On nOS](../self-host.md#on-nos)). Nothing to do before launch.
14. **Off-site backups:** optional, off for now. The encrypted dumps stay on the VPS. The runbook's [Off-site copies](../ops/runbook.md#off-site-copies) turns them on later.
15. **Legal:** filled in from the Nithron website's business details (controller, address, enterprise number, HostStronger in Germany, courts of West Flanders). The draft notices are gone, and CI now fails if a placeholder comes back.
16. **Support server:** later. Its invite goes where the listing docs say `[TO FILL: invite link]`, before applying for verification.
17. **Soft launch:** deploy the hosted bot and invite it to communities. Discord accepts verification from 75 servers; the limit for unverified bots is 100.
18. **Verification:** apply with [docs/listing/privileged-intents.md](../listing/privileged-intents.md), following [docs/listing/verification.md](../listing/verification.md).
19. **Release:** after approval, I tag `v0.1.0`, publish the release notes and the App Directory listing.
