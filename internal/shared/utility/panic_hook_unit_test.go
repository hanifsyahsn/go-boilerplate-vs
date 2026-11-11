package utility

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type hookKey struct{}

func TestReportPanic_U(t *testing.T) {
	testCases := []struct {
		name string
		run  func(t *testing.T)
	}{
		{name: "with no hook set, ReportPanic does nothing", run: func(t *testing.T) {
			panicHook.Store(nil)
			ReportPanic(context.Background(), "boom")
		}},
		{name: "the hook gets the ctx and the recovered value", run: func(t *testing.T) {
			var gotCtx context.Context
			var got any
			SetPanicHook(func(ctx context.Context, recovered any) { gotCtx, got = ctx, recovered })
			ctx := context.WithValue(context.Background(), hookKey{}, "mine")
			ReportPanic(ctx, "boom")
			require.Equal(t, "boom", got)
			require.Equal(t, "mine", gotCtx.Value(hookKey{}))
		}},
		{name: "a panic in a SafeGoContext goroutine reaches the hook", run: func(t *testing.T) {
			got := make(chan any, 1)
			SetPanicHook(func(_ context.Context, recovered any) { got <- recovered })
			SafeGoContext(context.Background(), func(context.Context) { panic("from a goroutine") })
			select {
			case r := <-got:
				require.Equal(t, "from a goroutine", r)
			case <-time.After(2 * time.Second):
				t.Fatal("the hook was never called")
			}
		}},
	}
	prevLog := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&syncBuffer{}, nil)))
	defer slog.SetDefault(prevLog)
	defer panicHook.Store(nil)
	for _, tc := range testCases {
		t.Run(tc.name, tc.run)
	}
}
