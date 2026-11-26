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

**Results:**

| Measure | Value |
|---|---|
| Events handled | 720,596 (100 a second, including setup and drain) |
| Handler time (synchronous part) | p50 8µs, p99 26µs, max 2.877ms |
| Heap after GC | 106.3 MB after warm-up, 103.4 MB at the end |
| Goroutines | 5023 after warm-up, 4328 at the end |
| CPU | 0.04 cores on average |
| Discord REST calls (fake) | 465,911 |
| Warnings / errors logged | 0 / 0 |
| Result | **PASS** |

Every 10 minutes:

```
10m0s     100 ev/s  p50 8µs       p99 18µs      heap  163.5 MB  goroutines  5073  REST   39050  warn 0  err 0
   20m0s     100 ev/s  p50 9µs       p99 31µs      heap  115.9 MB  goroutines  5056  REST   78225  warn 0  err 0
   30m0s     100 ev/s  p50 8µs       p99 19µs      heap  208.6 MB  goroutines  5026  REST  116831  warn 0  err 0
   40m0s     100 ev/s  p50 8µs       p99 22µs      heap  184.2 MB  goroutines  5089  REST  155886  warn 0  err 0
   50m0s     100 ev/s  p50 8µs       p99 19µs      heap  125.4 MB  goroutines  5108  REST  194455  warn 0  err 0
  1h0m0s     100 ev/s  p50 8µs       p99 21µs      heap  193.3 MB  goroutines  5042  REST  233531  warn 0  err 0
 1h10m0s     100 ev/s  p50 8µs       p99 17µs      heap  155.9 MB  goroutines  5057  REST  272705  warn 0  err 0
 1h20m0s     100 ev/s  p50 8µs       p99 17µs      heap  172.3 MB  goroutines  5054  REST  312248  warn 0  err 0
 1h30m0s     100 ev/s  p50 8µs       p99 20µs      heap  167.3 MB  goroutines  5067  REST  351006  warn 0  err 0
 1h40m0s     100 ev/s  p50 8µs       p99 17µs      heap  214.9 MB  goroutines  5034  REST  389679  warn 0  err 0
 1h50m0s     100 ev/s  p50 9µs       p99 27µs      heap  151.3 MB  goroutines  5044  REST  427420  warn 0  err 0
  2h0m0s     100 ev/s  p50 9µs       p99 33µs      heap  165.8 MB  goroutines  5044  REST  465804  warn 0  err 0
```

**Reading the numbers:**

- **Handler time** is the synchronous part of each gateway event, which is what holds up the shard's event loop. Slow work (posting log entries, Guardian actions, database writes) runs in background workers.
- **Goroutines:** most of the roughly 5,000 are the server log's per-channel workers. Each keeps a channel's log entries in order and exits after a minute idle, so the count follows the number of channels with recent activity, not the number of servers.
- **Memory:** the live heap swings between about 100 and 200 MB between collections, and returns to the same level after GC. With Postgres, Valkey and the OS, a 4 GB VPS leaves ample room at this scale, and 2 GB is enough for a few hundred servers (self-host guide).
- **Not measured here:** Discord's own rate limits and network latency. The fake API answers at once.

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
