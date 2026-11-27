package store_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/nithron/niro/internal/core"
	"github.com/nithron/niro/internal/core/message"
	"github.com/nithron/niro/internal/modules/moderation"
	"github.com/nithron/niro/internal/modules/roles"
	"github.com/nithron/niro/internal/store"
	"github.com/nithron/niro/internal/testutil"
)

// TestBackupRestore runs the backup service's pg_dump (custom format) and the
// restore from docs/ops/backup-restore.md, and checks the restored database
// holds the same data and works with the current migrations.
func TestBackupRestore(t *testing.T) {
	testutil.RequireDocker(t)
	ctx := context.Background()
	ctr, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("niro"), postgres.WithUsername("niro"), postgres.WithPassword("niro"),
		postgres.BasicWaitStrategies())
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatal(err)
	}
	open := func(db string) *store.Store {
		url, err := ctr.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			t.Fatal(err)
		}
		pool, err := store.Connect(ctx, strings.Replace(url, "/niro?", "/"+db+"?", 1), 30*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(pool.Close)
		if _, err := store.Migrate(ctx, pool); err != nil {
			t.Fatal(err)
		}
		return store.New(pool)
	}
	exec := func(cmd ...string) string {
		code, out, err := ctr.Exec(ctx, cmd)
		b, _ := io.ReadAll(out)
		if err != nil || code != 0 {
			t.Fatalf("%v: exit %d, %v: %s", cmd, code, err, b)
		}
		return string(b)
	}

	// Seed a server with settings, a case and a role panel.
	src := open("niro")
	if err := src.UpsertGuild(ctx, core.GuildInfo{ID: 1, Name: "G", OwnerID: 9}); err != nil {
		t.Fatal(err)
	}
	actor := snow(9)
	if _, err := src.SetModuleEnabled(ctx, 1, core.ModuleLogging, true, &actor, core.SourceDashboard); err != nil {
		t.Fatal(err)
	}
	if _, err := src.CreateCase(ctx, moderation.NewCase{GuildID: 1, Action: moderation.ActionWarn, TargetID: 50, ModeratorID: 9, Reason: "spam", Source: moderation.SourceManual}); err != nil {
		t.Fatal(err)
	}
	if _, err := src.CreatePanel(ctx, roles.Panel{GuildID: 1, Name: "P", ChannelID: 5, Message: message.Simple("x"),
		Style: roles.StyleButtons, Mode: roles.ModeToggle, Roles: []roles.PanelRole{{RoleID: 10, Label: "R"}}}); err != nil {
		t.Fatal(err)
	}

	// Back up the way deploy/backup/backup.sh does, then restore into an empty database.
	exec("pg_dump", "-U", "niro", "-d", "niro", "--format=custom", "--no-owner", "--file=/tmp/niro.dump")
	exec("createdb", "-U", "niro", "niro_restored")
	exec("pg_restore", "-U", "niro", "-d", "niro_restored", "--no-owner", "/tmp/niro.dump")

	dst := open("niro_restored") // migrating again must be a no-op
	c, err := dst.GetCase(ctx, 1, 1)
	if err != nil || c.Reason != "spam" {
		t.Fatalf("restored case = %+v, %v", c, err)
	}
	panels, err := dst.ListPanels(ctx, 1)
	if err != nil || len(panels) != 1 {
		t.Fatalf("restored panels = %d, %v", len(panels), err)
	}
	states, err := dst.ListModuleStates(ctx, 1)
	if err != nil || len(states) == 0 {
		t.Fatalf("restored module states = %v, %v", states, err)
	}
	counts := func(db string) string {
		return exec("psql", "-U", "niro", "-d", db, "-Atc",
			"select (select count(*) from guilds), (select count(*) from mod_cases), (select count(*) from config_audit), (select max(version_id) from goose_db_version)")
	}
	if a, b := counts("niro"), counts("niro_restored"); a != b {
		t.Fatalf("row counts differ: %q vs %q", a, b)
	}
}
