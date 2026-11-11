// Package notifier sends operational alerts (a panic, a background job that
// failed) to the people running the service (decision 0011): a Notifier
// interface, Telegram as the first implementation, Noop when unconfigured.
package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Level is how bad an alert is.
type Level string

const (
	LevelError Level = "error"
	LevelWarn  Level = "warn"
)

// Alert is one operational event.
type Alert struct {
	Level Level
	// Title is short and stable — it is the dedupe key ("panic", "cleanup
	// failed: auth_tokens").
	Title string
	// Text is the details; never a body, token, password or email address.
	Text string
	// RequestID is optional.
	RequestID string
}

// Notifier takes an alert. Notify never blocks and never returns an error:
// alerting must not hurt the code that alerts.
type Notifier interface {
	Notify(ctx context.Context, a Alert)
}

// Noop drops every alert (no TELEGRAM_* configured).
type Noop struct{}

func NewNoop() Noop { return Noop{} }

func (Noop) Notify(context.Context, Alert) {}

const (
	// queueSize alerts wait for the sender; past it they are dropped.
	queueSize = 100
	// dedupeWindow sends one alert per title at most this often; the ones in
	// between are counted into the next one.
	dedupeWindow = 5 * time.Minute
	// sendTimeout bounds one Telegram call.
	sendTimeout = 5 * time.Second
	// defaultAPIBase is Telegram's Bot API.
	defaultAPIBase = "https://api.telegram.org"
)

// Telegram sends alerts to one chat through a bot, asynchronously: Notify
// queues, Run (in main's background group) sends. The bot token never
// reaches a log.
type Telegram struct {
	token, chatID        string
	environment, service string

	queue  chan Alert
	client *http.Client

	mu         sync.Mutex
	lastSent   map[string]time.Time
	suppressed map[string]int

	// Seams for tests.
	apiBase string
	now     func() time.Time
}

// NewTelegram builds the notifier; environment and service head every
// message ("[production] go_boilerplate_vs — ERROR: …").
func NewTelegram(botToken, chatID, environment, service string) *Telegram {
	return &Telegram{
		token: botToken, chatID: chatID, environment: environment, service: service,
		queue:      make(chan Alert, queueSize),
		client:     &http.Client{Timeout: sendTimeout},
		lastSent:   map[string]time.Time{},
		suppressed: map[string]int{},
		apiBase:    defaultAPIBase,
		now:        time.Now,
	}
}

// Notify queues a for the sender. The same title is sent at most once per
// dedupeWindow: the ones in between are counted and the next one sent says
// "(N more since)". A full queue drops the alert (logged).
func (t *Telegram) Notify(ctx context.Context, a Alert) {
	t.mu.Lock()
	now := t.now()
	if last, ok := t.lastSent[a.Title]; ok && now.Sub(last) < dedupeWindow {
		t.suppressed[a.Title]++
		t.mu.Unlock()
		return
	}
	t.lastSent[a.Title] = now
	if n := t.suppressed[a.Title]; n > 0 {
		a.Text += fmt.Sprintf("\n(%d more since)", n)
		delete(t.suppressed, a.Title)
	}
	t.mu.Unlock()

	select {
	case t.queue <- a:
	default:
		slog.WarnContext(ctx, "notifier: queue full, alert dropped", "title", a.Title)
	}
}

// Run sends queued alerts until ctx is done, then sends what is still queued
// and returns. Each send outlives a cancel (an alert raised during shutdown
// is still delivered), bounded by the client's timeout.
func (t *Telegram) Run(ctx context.Context) {
	sendCtx := context.WithoutCancel(ctx)
	for {
		select {
		case a := <-t.queue:
			t.send(sendCtx, a)
		case <-ctx.Done():
			for {
				select {
				case a := <-t.queue:
					t.send(sendCtx, a)
				default:
					return
				}
			}
		}
	}
}

// message is the alert as Telegram shows it (plain text, no parse_mode).
func (t *Telegram) message(a Alert) string {
	msg := "[" + t.environment + "] " + t.service + " — " + strings.ToUpper(string(a.Level)) + ": " + a.Title + "\n" + a.Text
	if a.RequestID != "" {
		msg += "\nrequest_id: " + a.RequestID
	}
	return msg
}

func (t *Telegram) send(ctx context.Context, a Alert) {
	body, _ := json.Marshal(map[string]any{ // a map of strings and a bool always marshals
		"chat_id":                  t.chatID,
		"text":                     t.message(a),
		"disable_web_page_preview": true,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.apiBase+"/bot"+t.token+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		slog.ErrorContext(ctx, "notifier: send failed", "status", 0, "err", t.redact(err))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		// A *url.Error carries the URL, which carries the token.
		slog.ErrorContext(ctx, "notifier: send failed", "status", 0, "err", t.redact(err))
		return
	}
	_ = resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		slog.ErrorContext(ctx, "notifier: send failed", "status", resp.StatusCode, "err", "telegram answered "+resp.Status)
	}
}

// redact is err's text with the bot token replaced by ***.
func (t *Telegram) redact(err error) string {
	if t.token == "" {
		return err.Error()
	}
	return strings.ReplaceAll(err.Error(), t.token, "***")
}
