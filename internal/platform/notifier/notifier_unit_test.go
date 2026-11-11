package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/logging"
	"github.com/stretchr/testify/require"
)

const token = "123456:SECRET-BOT-TOKEN"

// syncBuffer is a bytes.Buffer safe for the logger of several goroutines.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// telegramServer records the sendMessage bodies and answers with status.
type telegramServer struct {
	*httptest.Server
	mu     sync.Mutex
	paths  []string
	bodies []map[string]any
}

func newTelegramServer(t *testing.T, status int) *telegramServer {
	t.Helper()
	s := &telegramServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(b, &body)
		s.mu.Lock()
		s.paths = append(s.paths, r.URL.Path)
		s.bodies = append(s.bodies, body)
		s.mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *telegramServer) texts() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, b := range s.bodies {
		out = append(out, b["text"].(string))
	}
	return out
}

// runUntilSent notifies, then runs the sender until the queue is empty.
func drain(tg *Telegram) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tg.Run(ctx)
}

func TestTelegram_U(t *testing.T) {
	now := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)

	testCases := []struct {
		name   string
		status int
		run    func(t *testing.T, tg *Telegram, srv *telegramServer, clock *time.Time, logs *syncBuffer)
	}{
		{name: "the message: environment, service, level, title, text and request id, plain text, no link preview", status: 200,
			run: func(t *testing.T, tg *Telegram, srv *telegramServer, _ *time.Time, _ *syncBuffer) {
				tg.Notify(context.Background(), Alert{Level: LevelError, Title: "panic", Text: "boom", RequestID: "req-1"})
				drain(tg)
				require.Equal(t, []string{"[production] go_boilerplate_vs — ERROR: panic\nboom\nrequest_id: req-1"}, srv.texts())
				require.Equal(t, "/bot"+token+"/sendMessage", srv.paths[0])
				require.Equal(t, "-100200", srv.bodies[0]["chat_id"])
				require.Equal(t, true, srv.bodies[0]["disable_web_page_preview"])
				require.NotContains(t, srv.bodies[0], "parse_mode")
			}},
		{name: "a warning without a request id", status: 200,
			run: func(t *testing.T, tg *Telegram, srv *telegramServer, _ *time.Time, _ *syncBuffer) {
				tg.Notify(context.Background(), Alert{Level: LevelWarn, Title: "cleanup failed: auth_tokens", Text: "db down"})
				drain(tg)
				require.Equal(t, []string{"[production] go_boilerplate_vs — WARN: cleanup failed: auth_tokens\ndb down"}, srv.texts())
			}},
		{name: "the same title within 5 min is sent once; the next one after says how many were held back", status: 200,
			run: func(t *testing.T, tg *Telegram, srv *telegramServer, clock *time.Time, _ *syncBuffer) {
				for range 4 {
					tg.Notify(context.Background(), Alert{Level: LevelError, Title: "panic", Text: "boom"})
				}
				tg.Notify(context.Background(), Alert{Level: LevelError, Title: "other", Text: "x"})
				*clock = clock.Add(5 * time.Minute)
				tg.Notify(context.Background(), Alert{Level: LevelError, Title: "panic", Text: "again"})
				drain(tg)
				require.Equal(t, []string{
					"[production] go_boilerplate_vs — ERROR: panic\nboom",
					"[production] go_boilerplate_vs — ERROR: other\nx",
					"[production] go_boilerplate_vs — ERROR: panic\nagain\n(3 more since)",
				}, srv.texts())
			}},
		{name: "a full queue drops the alert and logs it", status: 200,
			run: func(t *testing.T, tg *Telegram, srv *telegramServer, _ *time.Time, logs *syncBuffer) {
				tg.queue = make(chan Alert, 1)
				tg.Notify(context.Background(), Alert{Title: "a"})
				tg.Notify(context.Background(), Alert{Title: "b"}) // nothing drains yet: dropped
				drain(tg)
				require.Len(t, srv.texts(), 1)
				require.Contains(t, logs.String(), `"msg":"notifier: queue full, alert dropped"`)
			}},
		{name: "a 500 from Telegram is logged with the status, without the token", status: 500,
			run: func(t *testing.T, tg *Telegram, _ *telegramServer, _ *time.Time, logs *syncBuffer) {
				tg.Notify(context.Background(), Alert{Title: "a"})
				drain(tg)
				require.Contains(t, logs.String(), `"msg":"notifier: send failed"`)
				require.Contains(t, logs.String(), `"status":500`)
				require.NotContains(t, logs.String(), "SECRET-BOT-TOKEN")
			}},
		{name: "a transport error carries the URL; the log has the token as ***", status: 200,
			run: func(t *testing.T, tg *Telegram, srv *telegramServer, _ *time.Time, logs *syncBuffer) {
				srv.Close() // nothing answers now
				tg.Notify(context.Background(), Alert{Title: "a"})
				drain(tg)
				require.Contains(t, logs.String(), `"msg":"notifier: send failed"`)
				require.Contains(t, logs.String(), "/bot***/sendMessage")
				require.NotContains(t, logs.String(), "SECRET-BOT-TOKEN")
			}},
		{name: "an unusable API URL is logged without the token", status: 200,
			run: func(t *testing.T, tg *Telegram, _ *telegramServer, _ *time.Time, logs *syncBuffer) {
				tg.apiBase = "http://bad host"
				tg.Notify(context.Background(), Alert{Title: "a"})
				drain(tg)
				require.Contains(t, logs.String(), `"msg":"notifier: send failed"`)
				require.NotContains(t, logs.String(), "SECRET-BOT-TOKEN")
			}},
		{name: "alerts queued before shutdown are still sent; Run keeps sending while live", status: 200,
			run: func(t *testing.T, tg *Telegram, srv *telegramServer, _ *time.Time, _ *syncBuffer) {
				ctx, cancel := context.WithCancel(context.Background())
				done := make(chan struct{})
				go func() { tg.Run(ctx); close(done) }()
				tg.Notify(context.Background(), Alert{Title: "live"})
				require.Eventually(t, func() bool { return len(srv.texts()) == 1 }, 2*time.Second, 5*time.Millisecond)
				cancel()
				<-done
			}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logs := &syncBuffer{}
			l, err := logging.New(logs, logging.FormatJSON, "debug")
			require.NoError(t, err)
			prev := slog.Default()
			slog.SetDefault(l)
			defer slog.SetDefault(prev)

			srv := newTelegramServer(t, tc.status)
			tg := NewTelegram(token, "-100200", "production", "go_boilerplate_vs")
			tg.apiBase = srv.URL
			clock := now
			tg.now = func() time.Time { return clock }
			tc.run(t, tg, srv, &clock, logs)
		})
	}
}

func TestNoop_U(t *testing.T) {
	testCases := []struct{ name string }{{name: "Noop takes an alert and does nothing"}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var n Notifier = NewNoop()
			n.Notify(context.Background(), Alert{Title: "x"})
		})
	}
}

func TestTelegram_Redact_U(t *testing.T) {
	testCases := []struct {
		name  string
		token string
		err   error
		want  string
	}{
		{name: "the token is replaced by ***", token: "abc:XYZ", err: io.ErrUnexpectedEOF, want: io.ErrUnexpectedEOF.Error()},
		{name: "a token inside the message", token: "abc:XYZ", err: &json.SyntaxError{}, want: (&json.SyntaxError{}).Error()},
		{name: "no token: the text as is (never *** between every character)", token: "", err: io.EOF, want: "EOF"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, NewTelegram(tc.token, "1", "e", "s").redact(tc.err))
		})
	}
}
