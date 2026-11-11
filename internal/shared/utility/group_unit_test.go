package utility

import (
	"bytes"
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type ctxKey struct{}

func TestGroup_U(t *testing.T) {
	testCases := []struct {
		name string
		run  func(t *testing.T, g *Group)
	}{
		{name: "Wait returns after every goroutine returned", run: func(t *testing.T, g *Group) {
			var finished atomic.Int32
			for range 5 {
				g.Go(context.Background(), func(context.Context) {
					time.Sleep(20 * time.Millisecond)
					finished.Add(1)
				})
			}
			g.Wait()
			require.Equal(t, int32(5), finished.Load())
		}},
		{name: "a panic is recovered and Wait still returns", run: func(t *testing.T, g *Group) {
			var logs syncBuffer
			prev := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
			defer slog.SetDefault(prev)

			var finished atomic.Bool
			g.Go(context.Background(), func(context.Context) { panic("boom") })
			g.Go(context.Background(), func(context.Context) { finished.Store(true) })

			done := make(chan struct{})
			go func() { g.Wait(); close(done) }()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("Wait never returned after a panic")
			}
			require.True(t, finished.Load(), "the other goroutine still ran")
			require.Eventually(t, func() bool { return bytes.Contains([]byte(logs.String()), []byte("panic recovered")) },
				time.Second, 5*time.Millisecond, "the panic is logged")
		}},
		{name: "the ctx passed in is the one fn receives", run: func(t *testing.T, g *Group) {
			ctx := context.WithValue(context.Background(), ctxKey{}, "mine")
			var got any
			g.Go(ctx, func(ctx context.Context) { got = ctx.Value(ctxKey{}) })
			g.Wait()
			require.Equal(t, "mine", got)
		}},
		{name: "Wait with nothing started returns at once", run: func(t *testing.T, g *Group) {
			g.Wait()
		}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.run(t, &Group{})
		})
	}
}
