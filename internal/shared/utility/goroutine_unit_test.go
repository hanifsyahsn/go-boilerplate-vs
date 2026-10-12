package utility

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/logging"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/requestid"
	"github.com/stretchr/testify/require"
)

// syncBuffer is a bytes.Buffer safe to write from the goroutine and read
// from the test.
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

func TestSafeGoContext_U(t *testing.T) {
	testCases := []struct {
		name      string
		ctx       context.Context
		fn        func(ctx context.Context, got chan<- string)
		wantPanic string // the panic value to find in the log; "" => fn mustn't panic
		wantID    string
	}{
		{
			name: "fn gets the context",
			ctx:  requestid.With(context.Background(), "abc"),
			fn:   func(ctx context.Context, got chan<- string) { got <- requestid.FromContext(ctx) },
		},
		{
			name:      "a panic is logged with the request id",
			ctx:       requestid.Detach(requestid.With(context.Background(), "req-9")),
			fn:        func(context.Context, chan<- string) { panic("boom-with-id") },
			wantPanic: "boom-with-id",
			wantID:    "req-9",
		},
		{
			name:      "a panic without an id is still logged",
			ctx:       context.Background(),
			fn:        func(context.Context, chan<- string) { panic("boom-no-id") },
			wantPanic: "boom-no-id",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf syncBuffer
			l, err := logging.New(&buf, logging.FormatJSON, "info")
			require.NoError(t, err)
			prev := slog.Default()
			slog.SetDefault(l)
			defer slog.SetDefault(prev)

			got := make(chan string, 1)
			SafeGoContext(tc.ctx, func(ctx context.Context) { tc.fn(ctx, got) })

			if tc.wantPanic == "" {
				select {
				case id := <-got:
					require.Equal(t, requestid.FromContext(tc.ctx), id)
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for fn")
				}
				return
			}

			// Find this case's record by its panic value: the TestSafeGo_*
			// goroutines may still be logging their own recovered panics.
			var rec map[string]any
			require.Eventually(t, func() bool {
				rec = findRecord(t, buf.String(), "panic", tc.wantPanic)
				return rec != nil
			}, time.Second, 5*time.Millisecond, buf.String())
			require.Equal(t, "ERROR", rec["level"])
			require.Equal(t, "panic recovered", rec["msg"])
			require.NotEmpty(t, rec["stack"])
			if tc.wantID != "" {
				require.Equal(t, tc.wantID, rec[requestid.Key])
			} else {
				require.NotContains(t, rec, requestid.Key)
			}
		})
	}
}

// findRecord returns the first JSON record in out whose key equals want.
func findRecord(t *testing.T, out, key, want string) map[string]any {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var rec map[string]any
		if json.Unmarshal([]byte(line), &rec) == nil && rec[key] == want {
			return rec
		}
	}
	return nil
}
