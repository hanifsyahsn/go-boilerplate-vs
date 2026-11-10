package testhelper

import (
	"log"
	"time"

	"github.com/testcontainers/testcontainers-go"
)

// startAttempts and startBackoff bound startContainer's retries; sleep is
// swapped out in tests.
var (
	startAttempts = 3
	startBackoff  = 2 * time.Second
	sleep         = time.Sleep
)

// startContainer starts a container, retrying a failed start. Every package
// of one `go test ./...` shares one testcontainers session and so one Ryuk
// reaper container; the packages that start first race to create it, and the
// losers fail with "wait for reaper ...: unexpected container status
// "created"" although the reaper comes up a moment later. A retry after a
// short wait finds it running. A container a failed attempt left behind is
// terminated first, so a retry never leaks one.
func startContainer(what string, start func() (testcontainers.Container, error)) (testcontainers.Container, error) {
	var err error
	for attempt := 1; ; attempt++ {
		var c testcontainers.Container
		c, err = start()
		if err == nil {
			return c, nil
		}
		_ = testcontainers.TerminateContainer(c) // a nil container is a no-op
		if attempt == startAttempts {
			return nil, err
		}
		log.Printf("starting the %s container failed (attempt %d of %d), retrying: %v", what, attempt, startAttempts, err)
		sleep(time.Duration(attempt) * startBackoff)
	}
}
