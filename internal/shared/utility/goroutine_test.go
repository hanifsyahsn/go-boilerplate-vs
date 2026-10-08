package utility

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSafeGo_RunsFunction(t *testing.T) {
	var called int32
	done := make(chan struct{})

	SafeGo(func() {
		atomic.StoreInt32(&called, 1)
		close(done)
	})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for SafeGo function to run")
	}

	if atomic.LoadInt32(&called) != 1 {
		t.Error("expected function to be called")
	}
}

func TestSafeGo_RecoversFromPanic(t *testing.T) {
	done := make(chan struct{})

	// If SafeGo does not recover, the panic will crash the whole test
	// binary (panics in goroutines are not recoverable by the caller),
	// so simply reaching the end of this test is itself proof that the
	// panic was contained.
	SafeGo(func() {
		defer close(done)
		panic("boom")
	})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for panicking goroutine to finish")
	}

	// Give the deferred recover() inside SafeGo a moment to run since it
	// executes after the inner defer that closes done.
	time.Sleep(10 * time.Millisecond)
}

func TestSafeGo_PanicDoesNotAffectOtherGoroutines(t *testing.T) {
	var wg sync.WaitGroup
	results := make([]int32, 10)

	for i := 0; i < 10; i++ {
		i := i
		wg.Add(1)
		SafeGo(func() {
			defer wg.Done()
			if i%2 == 0 {
				panic("boom")
			}
			atomic.StoreInt32(&results[i], 1)
		})
	}

	waitDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for goroutines to finish")
	}

	for i, v := range results {
		if i%2 == 0 {
			if v != 0 {
				t.Errorf("index %d: expected 0 (never set, panicked before set), got %d", i, v)
			}
			continue
		}
		if v != 1 {
			t.Errorf("index %d: expected function to complete and set 1, got %d", i, v)
		}
	}
}

func TestSafeGo_MultiplePanicsDoNotCrashProcess(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		SafeGo(func() {
			defer wg.Done()
			panic("repeated panic")
		})
	}

	waitDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for panicking goroutines to finish")
	}
	// Reaching this line proves none of the panics crashed the test binary.
}
