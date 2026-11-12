package utility

import (
	"context"
	"sync"
)

// GoFunc starts fn in the background with ctx. Services that work off the
// request path (sending an email, …) take one: main wires Group.Go, tests
// pass one that runs fn inline.
type GoFunc func(ctx context.Context, fn func(ctx context.Context))

// Group runs background goroutines that main waits for at shutdown. A panic
// in one is recovered, logged and reported, as with SafeGoContext.
//
// main cancels the ctx it hands the goroutines only after the HTTP server has
// drained its requests, then Waits, then closes the DB pool — so work queued
// by the last requests is still done (decision 0005, D6).
type Group struct{ wg sync.WaitGroup }

// Go runs fn(ctx) in a goroutine Wait waits for. The goroutine is counted
// before it starts, so a Wait that follows Go never misses it, and counted
// done only after a panic in it was logged and reported: main exits right
// after Wait, which would otherwise cut that record (and its alert) off.
func (g *Group) Go(ctx context.Context, fn func(ctx context.Context)) {
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		runRecovered(ctx, fn)
	}()
}

// Wait returns once every goroutine started with Go has returned (or
// panicked).
func (g *Group) Wait() {
	g.wg.Wait()
}
