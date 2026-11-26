#!/bin/sh
# Nightly Postgres backup for Niro (deploy/compose.yml, service "backup").
#
# Writes a pg_dump (custom format) to /backups every day at BACKUP_HOUR (UTC),
# keeps BACKUP_KEEP_DAILY daily and BACKUP_KEEP_WEEKLY Sunday dumps, and
# records the last success for monitoring in /metrics/niro_backup.prom
# (node_exporter's textfile format). With BACKUP_AGE_RECIPIENT set, dumps are
# encrypted with age. Run once now with: docker compose exec backup sh /backup/backup.sh now
#
# Dumps contain personal data (cases, ticket transcripts). Keep them only as
# long as the retention above: the 30-day purge of servers that removed Niro
# relies on old dumps going away too.
set -eu
umask 077 # dumps hold personal data: readable by their owner only

: "${BACKUP_HOUR:=3}" "${BACKUP_KEEP_DAILY:=7}" "${BACKUP_KEEP_WEEKLY:=3}" "${BACKUP_AGE_RECIPIENT:=}"
OUT=/backups
METRICS=/metrics/niro_backup.prom

log() { echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) backup: $*"; }

backup_once() {
	stamp=$(date -u +%Y%m%d-%H%M%S)
	name="niro-daily-$stamp.dump"
	if [ "$(date -u +%u)" = 7 ]; then name="niro-weekly-$stamp.dump"; fi
	tmp="$OUT/.$name.partial"
	pg_dump --format=custom --no-owner --file="$tmp"
	if [ -n "$BACKUP_AGE_RECIPIENT" ]; then
		command -v age >/dev/null || apk add --no-cache age >/dev/null
		age --recipient "$BACKUP_AGE_RECIPIENT" --output "$tmp.age" "$tmp"
		rm -f "$tmp"
		tmp="$tmp.age"
		name="$name.age"
	fi
	mv "$tmp" "$OUT/$name"
	size=$(wc -c <"$OUT/$name")
	log "wrote $name ($size bytes)"
	prune niro-daily- "$BACKUP_KEEP_DAILY"
	prune niro-weekly- "$BACKUP_KEEP_WEEKLY"
	mkdir -p "$(dirname "$METRICS")"
	cat >"$METRICS.tmp" <<EOF
# HELP niro_backup_last_success_timestamp_seconds When the last database backup finished.
# TYPE niro_backup_last_success_timestamp_seconds gauge
niro_backup_last_success_timestamp_seconds $(date -u +%s)
# HELP niro_backup_last_size_bytes Size of the last database backup.
# TYPE niro_backup_last_size_bytes gauge
niro_backup_last_size_bytes $size
EOF
	mv "$METRICS.tmp" "$METRICS"
}

# prune keeps the newest $2 files starting with $1.
prune() {
	ls -1 "$OUT" | grep "^$1" | sort -r | tail -n +"$(($2 + 1))" | while read -r old; do
		rm -f "$OUT/$old"
		log "removed $old"
	done
}

mkdir -p "$OUT"
if [ "${1:-}" = now ]; then
	backup_once
	exit 0
fi

log "next backups at $BACKUP_HOUR:00 UTC daily"
while true; do
	now=$(date -u +%s)
	next=$(date -u -d "$(date -u +%Y-%m-%d) $(printf %02d "$BACKUP_HOUR"):00:00" +%s)
	if [ "$next" -le "$now" ]; then next=$((next + 86400)); fi
	sleep $((next - now))
	backup_once || log "FAILED (see above); the monitoring alert fires if this keeps happening"
done
