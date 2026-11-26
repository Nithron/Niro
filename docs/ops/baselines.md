# Load-test and resilience baselines

Measured on 2026-11-26 with the `v0.1.0` candidate (M5 branch). Re-run after changes to the bot's event handling, and compare.

## Soak test

`make soak` (`cmd/niro-soak`) runs Niro's real event handlers with real Postgres and Valkey, against synthetic gateway traffic and a fake Discord REST API on localhost.

**Run:**

- **Servers:** 2,500, each with 5 channels and 20 members. Guardian, logging and welcome are on in all of them.
- **Traffic:** 100 gateway events a second for 2 hours, after a 5-minute warm-up:
  - mostly messages, some with links and scam links;
  - 8% edits, 4% deletes, 1% joins, 0.5% leaves;
  - a 50-join raid burst every 10 minutes.
- **Machine:** the development workstation, with Postgres 18 and Valkey 8 in Docker on the same machine.

RESULTS_PENDING

## Shard resume

**Setup:** Niro Dev in its test server, running `niro all` in Docker. The container's internet link was cut with `docker network disconnect` while Postgres and Valkey stayed reachable. Gateway events were read from `niro_bot_gateway_events_total`.

| Outage | What happened | Events missed |
|---|---|---|
| 8 s | Nothing: the TCP connection survived and no reconnect was needed | None |
| 65 s | disgo noticed the dead connection after 82 s (two missed heartbeats). After the network returned, the session **resumed** (`Resumed` +1, no new `Ready`). | None. Discord replays missed events on resume |
| about 2.5 min | Discord had invalidated the session (`invalid session, can_resume=false`). disgo waited out its reconnect backoff (32 s at that point), then started a new session (`Ready` +1, `GuildReady` +1). Back about 1.5 minutes after the network returned | Events during the outage are lost: joins aren't checked by Guardian, and edits and deletes aren't logged |

What follows from this:

- **Short network blips are harmless.** Resumes replay what was missed.
- **Long outages, a minute and a half or more, lose events.** That is Discord's behaviour, not a Niro choice.
- **`NiroShardDown` (5 minutes) does not fire for these cases.** It fires when a shard stays down. `NiroGatewayReconnecting` catches repeated new sessions.
- **The bot's `/readyz`** reports not ready while a shard is down (`503`), and ready again after it reconnects.
