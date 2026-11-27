#!/bin/sh
# Copies the nightly database dumps off the server with restic, an hour after
# the backup service writes them, and keeps 28 days (the same limit as the
# local copies; see docs/ops/backup-restore.md). Configure the repository in
# deploy/hosted/offsite.env (RESTIC_REPOSITORY, RESTIC_PASSWORD and the
# storage credentials). Run once now: docker compose exec offsite sh /offsite/offsite.sh now
set -u
: "${OFFSITE_HOUR:=4}"
case "$OFFSITE_HOUR" in [0-9] | 0[0-9] | 1[0-9] | 2[0-3]) ;; *) echo "OFFSITE_HOUR must be 0-23" >&2; exit 1 ;; esac
OFFSITE_HOUR=$(echo "$OFFSITE_HOUR" | sed 's/^0\([0-9]\)$/\1/')
METRICS=/metrics/niro_offsite.prom
log() { echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) offsite: $*"; }

once() {
	restic cat config >/dev/null 2>&1 || restic init || { log "cannot open or create the repository"; return 1; }
	restic backup --host niro --tag dumps --exclude '.*.partial' /backups || { log "restic backup failed"; return 1; }
	restic forget --host niro --tag dumps --keep-within 28d --prune || { log "pruning old copies failed"; return 1; }
	cat >"$METRICS.tmp" <<EOF
# HELP niro_offsite_last_success_timestamp_seconds When the dumps were last copied off the server.
# TYPE niro_offsite_last_success_timestamp_seconds gauge
niro_offsite_last_success_timestamp_seconds $(date -u +%s)
EOF
	mv "$METRICS.tmp" "$METRICS"
	log "copied"
}

if [ "${1:-}" = now ]; then once; exit $?; fi
while true; do
	now=$(date -u +%s)
	next=$(date -u -d "$(date -u +%Y-%m-%d) $OFFSITE_HOUR:00:00" +%s) || exit 1
	if [ "$next" -le "$now" ]; then next=$((next + 86400)); fi
	sleep $((next - now))
	once || log "FAILED (see above)"
done
