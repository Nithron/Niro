#!/bin/sh
# Nightly Postgres backup for Niro (deploy/compose.yml, service "backup").
#
# Writes a pg_dump (custom format) to /backups every day at BACKUP_HOUR (UTC),
# and once at start if there is no dump yet. Keeps BACKUP_KEEP_DAILY daily and
# BACKUP_KEEP_WEEKLY Sunday dumps, and records the last success for monitoring
# in /metrics/niro_backup.prom (node_exporter's textfile format). With
# BACKUP_AGE_RECIPIENT set, dumps are encrypted with age and never written to
# the backup folder unencrypted.
# Run once now: docker compose exec backup sh /backup/backup.sh now
#
# Dumps contain personal data (cases, ticket transcripts). Keep them only as
# long as the retention above: the 30-day purge of servers that removed Niro
# relies on old dumps going away too.
#
# Every step checks its own result (set -e does not apply inside a function
# called from an || list), so a failed run never prunes good dumps or reports
# success.
set -u
umask 077 # dumps hold personal data: readable by their owner only

: "${BACKUP_HOUR:=3}" "${BACKUP_KEEP_DAILY:=7}" "${BACKUP_KEEP_WEEKLY:=3}" "${BACKUP_AGE_RECIPIENT:=}"
OUT=/backups
WORK=/tmp/niro-backup
METRICS=/metrics/niro_backup.prom

log() { echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) backup: $*"; }
die() { log "$*"; exit 1; }

# Validate the settings once, at start.
case "$BACKUP_HOUR" in [0-9] | 0[0-9] | 1[0-9] | 2[0-3]) ;; *) die "BACKUP_HOUR must be 0-23, got '$BACKUP_HOUR'" ;; esac
BACKUP_HOUR=$(echo "$BACKUP_HOUR" | sed 's/^0\([0-9]\)$/\1/')
for n in "$BACKUP_KEEP_DAILY" "$BACKUP_KEEP_WEEKLY"; do
	case "$n" in '' | *[!0-9]*) die "BACKUP_KEEP_DAILY and BACKUP_KEEP_WEEKLY must be numbers" ;; esac
done
if [ -n "$BACKUP_AGE_RECIPIENT" ] && ! command -v age >/dev/null; then
	apk add --no-cache age >/dev/null 2>&1 || die "BACKUP_AGE_RECIPIENT is set but age could not be installed; refusing to write unencrypted dumps"
fi
mkdir -p "$OUT" "$WORK" "$(dirname "$METRICS")" || die "cannot create the backup folders"

backup_once() {
	rm -f "$WORK"/* # leftovers of an interrupted run
	stamp=$(date -u +%Y%m%d-%H%M%S)
	kind=daily
	if [ "$(date -u +%u)" = 7 ]; then kind=weekly; fi
	name="niro-$kind-$stamp.dump"
	dump="$WORK/$name"

	pg_dump --format=custom --no-owner --file="$dump" || { log "pg_dump failed"; return 1; }
	[ -s "$dump" ] || { log "the dump is empty"; return 1; }
	pg_restore --list "$dump" >/dev/null || { log "the dump is unreadable"; return 1; }
	if [ -n "$BACKUP_AGE_RECIPIENT" ]; then
		age --recipient "$BACKUP_AGE_RECIPIENT" --output "$dump.age" "$dump" || { log "encrypting failed"; return 1; }
		rm -f "$dump"
		dump="$dump.age"
		name="$name.age"
	fi
	# The work folder sits on another filesystem than /backups, so copy to a
	# hidden name first and rename, so a half-copied file is never visible.
	cp "$dump" "$OUT/.$name.partial" || { log "copying the dump failed"; return 1; }
	mv "$OUT/.$name.partial" "$OUT/$name" || { log "publishing the dump failed"; return 1; }
	rm -f "$WORK"/*
	size=$(wc -c <"$OUT/$name")
	log "wrote $name ($size bytes)"

	prune niro-daily- "$BACKUP_KEEP_DAILY"
	prune niro-weekly- "$BACKUP_KEEP_WEEKLY"
	# The metrics file holds no personal data; node_exporter must read it.
	(umask 022 && {
		echo "# HELP niro_backup_last_success_timestamp_seconds When the last database backup finished."
		echo "# TYPE niro_backup_last_success_timestamp_seconds gauge"
		echo "niro_backup_last_success_timestamp_seconds $(date -u +%s)"
		echo "# HELP niro_backup_last_size_bytes Size of the last database backup."
		echo "# TYPE niro_backup_last_size_bytes gauge"
		echo "niro_backup_last_size_bytes $size"
	} >"$METRICS.tmp" && mv "$METRICS.tmp" "$METRICS")
}

# prune keeps the newest $2 dumps whose names start with $1, and removes
# partial copies left by an interrupted run.
prune() {
	find "$OUT" -maxdepth 1 -name '.*.partial' -mmin +60 -exec rm -f {} +
	ls -1 "$OUT" | grep "^$1" | sort -r | tail -n +"$(($2 + 1))" | while read -r old; do
		rm -f "$OUT/$old" && log "removed $old"
	done
}

if [ "${1:-}" = now ]; then
	backup_once
	exit $?
fi

if ! ls -1 "$OUT" | grep -q '^niro-'; then
	log "no dump yet: taking one now"
	backup_once || log "FAILED (see above)"
fi
log "next backups at $BACKUP_HOUR:00 UTC daily"
while true; do
	now=$(date -u +%s)
	next=$(date -u -d "$(date -u +%Y-%m-%d) $BACKUP_HOUR:00:00" +%s) || die "cannot compute the next run time"
	if [ "$next" -le "$now" ]; then next=$((next + 86400)); fi
	sleep $((next - now))
	backup_once || log "FAILED (see above); the monitoring alert fires if this keeps happening"
done
