// Package rpc lets the API ask the bot to do things only the bot can do
// (anything that needs the bot token). Requests go through a Valkey stream
// consumed by a consumer group, so any bot process can answer; replies come
// back on a per-request list the caller waits on.
package rpc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/valkey-io/valkey-go"
)

const (
	streamKey   = "nithro:rpc"
	group       = "bots"
	replyPrefix = "nithro:rpc:reply:"
	maxInFlight = 16
)

// Error codes returned to callers.
const (
	CodeNotFound    = "not_found"
	CodeInvalid     = "invalid"
	CodeDiscord     = "discord_error"
	CodeUnavailable = "unavailable"
	CodeInternal    = "internal"
)

// Error is a failure reported by the bot (or the transport).
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Errorf builds an Error for handlers to return.
func Errorf(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// IsCode reports whether err is an *Error with the given code.
func IsCode(err error, code string) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == code
}

// Request is one call.
type Request struct {
	ID       string          `json:"id"`
	Method   string          `json:"method"`
	GuildID  snowflake.ID    `json:"guild_id"`
	ActorID  snowflake.ID    `json:"actor_id"`
	Params   json.RawMessage `json:"params,omitempty"`
	Deadline int64           `json:"deadline"` // unix ms; the bot skips expired requests
}

// Bind decodes the request parameters.
func (r Request) Bind(v any) error {
	if len(r.Params) == 0 {
		return Errorf(CodeInvalid, "missing parameters")
	}
	if err := json.Unmarshal(r.Params, v); err != nil {
		return Errorf(CodeInvalid, "invalid parameters: %v", err)
	}
	return nil
}

type response struct {
	Data  json.RawMessage `json:"data,omitempty"`
	Error *Error          `json:"error,omitempty"`
}

// ---- client -----------------------------------------------------------------

// Client sends requests (used by the API).
type Client struct {
	v       valkey.Client
	timeout time.Duration
}

func NewClient(v valkey.Client, timeout time.Duration) *Client {
	return &Client{v: v, timeout: timeout}
}

// Call sends a request and decodes the reply into out (which may be nil).
// It fails with CodeUnavailable if no bot answers within the timeout.
func (c *Client) Call(ctx context.Context, method string, guildID, actorID snowflake.ID, params, out any) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	deadline, _ := ctx.Deadline()

	idBytes := make([]byte, 16)
	_, _ = rand.Read(idBytes)
	req := Request{ID: hex.EncodeToString(idBytes), Method: method, GuildID: guildID, ActorID: actorID, Deadline: deadline.UnixMilli()}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return err
		}
		req.Params = raw
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if err := c.v.Do(ctx, c.v.B().Xadd().Key(streamKey).Maxlen().Almost().Threshold("10000").Id("*").
		FieldValue().FieldValue("p", string(payload)).Build()).Error(); err != nil {
		return fmt.Errorf("sending rpc %s: %w", method, err)
	}

	wait := time.Until(deadline).Seconds()
	vals, err := c.v.Do(ctx, c.v.B().Blpop().Key(replyPrefix+req.ID).Timeout(max(wait, 0.1)).Build()).AsStrSlice()
	if valkey.IsValkeyNil(err) || errors.Is(err, context.DeadlineExceeded) || (err == nil && len(vals) < 2) {
		return &Error{Code: CodeUnavailable, Message: "the bot did not answer in time"}
	}
	if err != nil {
		return fmt.Errorf("waiting for rpc %s: %w", method, err)
	}
	var resp response
	if err := json.Unmarshal([]byte(vals[1]), &resp); err != nil {
		return fmt.Errorf("decoding rpc %s reply: %w", method, err)
	}
	if resp.Error != nil {
		return resp.Error
	}
	if out != nil && len(resp.Data) > 0 {
		return json.Unmarshal(resp.Data, out)
	}
	return nil
}

// ---- server -----------------------------------------------------------------

// HandlerFunc answers one method. Returning an *Error passes it to the
// caller as is; any other error is logged and reported as CodeInternal.
type HandlerFunc func(ctx context.Context, req Request) (any, error)

// Server consumes requests (run by every bot process).
type Server struct {
	v        valkey.Client
	consumer string
	log      *slog.Logger
	mu       sync.RWMutex
	handlers map[string]HandlerFunc
}

func NewServer(v valkey.Client, consumer string, log *slog.Logger) *Server {
	return &Server{v: v, consumer: consumer, log: log.With("component", "rpc"), handlers: map[string]HandlerFunc{}}
}

func (s *Server) Handle(method string, h HandlerFunc) {
	s.mu.Lock()
	s.handlers[method] = h
	s.mu.Unlock()
}

// Run consumes requests until ctx is done.
func (s *Server) Run(ctx context.Context) error {
	err := s.v.Do(ctx, s.v.B().XgroupCreate().Key(streamKey).Group(group).Id("$").Mkstream().Build()).Error()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return fmt.Errorf("creating rpc consumer group: %w", err)
	}
	sem := make(chan struct{}, maxInFlight)
	var wg sync.WaitGroup
	defer wg.Wait()
	for ctx.Err() == nil {
		res, err := s.v.Do(ctx, s.v.B().Xreadgroup().Group(group, s.consumer).Count(maxInFlight).Block(2000).
			Streams().Key(streamKey).Id(">").Build()).AsXRead()
		if ctx.Err() != nil {
			return nil
		}
		if valkey.IsValkeyNil(err) {
			continue
		}
		if err != nil {
			s.log.Warn("reading rpc stream failed", "err", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
			}
			continue
		}
		for _, entry := range res[streamKey] {
			sem <- struct{}{}
			wg.Go(func() {
				defer func() { <-sem }()
				s.serve(ctx, entry)
			})
		}
	}
	return nil
}

func (s *Server) serve(ctx context.Context, entry valkey.XRangeEntry) {
	defer func() {
		_ = s.v.Do(context.Background(), s.v.B().Xack().Key(streamKey).Group(group).Id(entry.ID).Build()).Error()
	}()
	var req Request
	if err := json.Unmarshal([]byte(entry.FieldValues["p"]), &req); err != nil || req.ID == "" {
		s.log.Warn("dropping malformed rpc request", "entry", entry.ID)
		return
	}
	deadline := time.UnixMilli(req.Deadline)
	if time.Now().After(deadline) {
		return // the caller has given up already
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	s.mu.RLock()
	h, ok := s.handlers[req.Method]
	s.mu.RUnlock()

	var resp response
	if !ok {
		resp.Error = Errorf(CodeNotFound, "unknown method %q", req.Method)
	} else if data, err := s.call(ctx, h, req); err != nil {
		var re *Error
		if errors.As(err, &re) {
			resp.Error = re
		} else {
			s.log.Error("rpc handler failed", "method", req.Method, "guild_id", req.GuildID, "err", err)
			resp.Error = Errorf(CodeInternal, "the bot could not complete the request")
		}
	} else if data != nil {
		raw, err := json.Marshal(data)
		if err != nil {
			resp.Error = Errorf(CodeInternal, "encoding reply: %v", err)
		} else {
			resp.Data = raw
		}
	}
	payload, _ := json.Marshal(resp)
	key := replyPrefix + req.ID
	for _, r := range s.v.DoMulti(context.Background(),
		s.v.B().Rpush().Key(key).Element(string(payload)).Build(),
		s.v.B().Expire().Key(key).Seconds(60).Build(),
	) {
		if err := r.Error(); err != nil {
			s.log.Warn("sending rpc reply failed", "err", err)
		}
	}
}

func (s *Server) call(ctx context.Context, h HandlerFunc, req Request) (data any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return h(ctx, req)
}
