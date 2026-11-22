package rpc_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcvalkey "github.com/testcontainers/testcontainers-go/modules/valkey"

	"github.com/nithron/niro/internal/cache"
	"github.com/nithron/niro/internal/rpc"
	"github.com/nithron/niro/internal/testutil"
)

func valkeyClient(t *testing.T) *cache.Client {
	t.Helper()
	testutil.RequireDocker(t)
	ctx := context.Background()
	ctr, err := tcvalkey.Run(ctx, "valkey/valkey:8-alpine")
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatal(err)
	}
	host, _ := ctr.Host(ctx)
	port, _ := ctr.MappedPort(ctx, "6379/tcp")
	c, err := cache.Connect(ctx, host+":"+port.Port(), "", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

type echo struct {
	Text string `json:"text"`
}

func TestRPC(t *testing.T) {
	kv := valkeyClient(t)
	ctx := t.Context()

	client := rpc.NewClient(kv.Raw(), 2*time.Second)
	// Before any server exists, calls time out cleanly.
	if err := client.Call(ctx, "echo", 1, 2, echo{"x"}, nil); !rpc.IsCode(err, rpc.CodeUnavailable) {
		t.Fatalf("no server: err = %v", err)
	}

	srv := rpc.NewServer(kv.Raw(), "test-1", slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv.Handle("echo", func(_ context.Context, r rpc.Request) (any, error) {
		var in echo
		if err := r.Bind(&in); err != nil {
			return nil, err
		}
		return echo{Text: in.Text + " from guild " + r.GuildID.String() + " by " + r.ActorID.String()}, nil
	})
	srv.Handle("fail", func(context.Context, rpc.Request) (any, error) {
		return nil, rpc.Errorf(rpc.CodeNotFound, "case #%d not found", 7)
	})
	srv.Handle("crash", func(context.Context, rpc.Request) (any, error) { panic("boom") })
	srv.Handle("internal", func(context.Context, rpc.Request) (any, error) { return nil, errors.New("secret detail") })
	go func() { _ = srv.Run(ctx) }()
	time.Sleep(300 * time.Millisecond) // let the consumer group exist

	var out echo
	if err := client.Call(ctx, "echo", 1, 2, echo{"hi"}, &out); err != nil || out.Text != "hi from guild 1 by 2" {
		t.Fatalf("echo = %+v, %v", out, err)
	}
	var re *rpc.Error
	if err := client.Call(ctx, "fail", 1, 2, nil, nil); !errors.As(err, &re) || re.Code != rpc.CodeNotFound || re.Message != "case #7 not found" {
		t.Fatalf("fail = %v", err)
	}
	if err := client.Call(ctx, "nope", 1, 2, nil, nil); !rpc.IsCode(err, rpc.CodeNotFound) {
		t.Fatalf("unknown method = %v", err)
	}
	if err := client.Call(ctx, "crash", 1, 2, nil, nil); !rpc.IsCode(err, rpc.CodeInternal) {
		t.Fatalf("panic = %v", err)
	}
	if err := client.Call(ctx, "internal", 1, 2, nil, nil); !errors.As(err, &re) || re.Code != rpc.CodeInternal || re.Message == "secret detail" {
		t.Fatalf("internal errors must not leak details: %v", err)
	}
	if err := client.Call(ctx, "echo", 1, 2, nil, nil); !rpc.IsCode(err, rpc.CodeInvalid) {
		t.Fatalf("missing params = %v", err)
	}
}
