# Backup and restore

## What is backed up

- **Postgres:** all of Niro's data. The `backup` service in [`deploy/compose.yml`](../../deploy/compose.yml) runs [`deploy/backup/backup.sh`](../../deploy/backup/backup.sh), which:
  - makes a `pg_dump` in custom format when it first starts with no dump yet, then every day at `BACKUP_HOUR` (UTC, default 03:00);
  - checks each dump (non-empty, readable by `pg_restore --list`) before publishing it. A failed run keeps every existing dump and doesn't record a success, so the alert fires;
  - writes it to `BACKUP_DIR` (default `deploy/backups/`), readable by its owner only;
  - keeps `BACKUP_KEEP_DAILY` daily dumps (7) and `BACKUP_KEEP_WEEKLY` Sunday dumps (3);
  - encrypts the dumps with [age](https://age-encryption.org) if `BACKUP_AGE_RECIPIENT` is set. Keep the private key **off** the server.
- **Valkey is not backed up.** It holds only sessions, rate-limit counters and short-lived caches. Losing it signs everyone out of the dashboard and empties the server log's edit/delete cache. No settings or cases are lost.

Retention is part of the privacy promise. Servers that remove Niro are deleted after 30 days, and no backup is older than 28 days. Don't keep dumps longer than this without changing the privacy policy.

## Off-site copies

A backup on the same disk doesn't survive losing the server. Copy `BACKUP_DIR` elsewhere after the nightly run, for example with restic or rclone to object storage, or rsync to another host. Use the same retention on the copy, or prune it to 28 days.

## Monitoring

Each run writes `niro_backup_last_success_timestamp_seconds` and `niro_backup_last_size_bytes` to the `backup-metrics` volume, in node_exporter's textfile format. [`deploy/monitoring/alerts.yml`](../../deploy/monitoring/alerts.yml) alerts:

- when no backup has succeeded for 26 hours (`NiroBackupMissing`);
- when none was ever recorded (`NiroBackupNeverRan`).

## Take a backup now

```sh
docker compose -f deploy/compose.yml exec backup sh /backup/backup.sh now
```

Do this before every upgrade.

## Restore

1. Stop Niro, so nothing writes during the restore:
   ```sh
   docker compose -f deploy/compose.yml stop niro
   ```
2. Pick the dump to restore. If it is encrypted, decrypt it first, on a machine that has the key:
   ```sh
   age -d -i key.txt niro-daily-….dump.age > niro.dump
   ```
3. Replace the database with the dump:
   ```sh
   docker compose -f deploy/compose.yml exec -T postgres dropdb -U niro --force niro
   docker compose -f deploy/compose.yml exec -T postgres createdb -U niro niro
   docker compose -f deploy/compose.yml exec -T postgres pg_restore -U niro -d niro --no-owner < niro.dump
   ```
4. Start Niro. It applies any migrations newer than the dump:
   ```sh
   docker compose -f deploy/compose.yml start niro
   ```
5. Check the logs for `bot online` and open the dashboard.
6. **Honour deletions made after the dump.** A dump from before an owner pressed **Delete data…**, or before Niro was removed, brings those servers back. When the bot starts, servers it is no longer in are marked as left. To delete them at once instead of after 30 days, run this once all shards are connected:
   ```sh
   docker compose -f deploy/compose.yml exec -T postgres psql -U niro -c "DELETE FROM guilds WHERE left_at > now() - interval '1 hour'"
   ```
   This errs on the side of deleting: a server that simply removed Niro loses its 30-day grace period.

**Restoring onto a new server:**

1. Do steps 1 to 4 of the [self-host guide](../self-host.md), but before step 4 start only the database: `docker compose -f deploy/compose.yml up -d postgres`.
2. Restore as above.
3. Start everything.

## Test restores

- **In CI:** `TestBackupRestore` (`internal/store/backup_integration_test.go`) runs on every change. It dumps a seeded database the way the backup service does, restores it into an empty one, checks the data and runs the migrations again.
- **On the hosted service:** once a month, restore the latest off-site copy into a scratch database and compare the row counts of `guilds`, `mod_cases` and `config_audit` with production:
  ```sh
  docker compose -f deploy/compose.yml exec -T postgres createdb -U niro niro_drill
  docker compose -f deploy/compose.yml exec -T postgres pg_restore -U niro -d niro_drill --no-owner < niro.dump
  docker compose -f deploy/compose.yml exec -T postgres psql -U niro -d niro_drill -c 'select count(*) from guilds'
  docker compose -f deploy/compose.yml exec -T postgres dropdb -U niro niro_drill
  ```
  Note the date and result in the runbook log.
