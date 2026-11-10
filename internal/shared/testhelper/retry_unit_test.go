package testhelper

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
)

func TestStartContainer_U(t *testing.T) {
	reaperRace := errors.New(`wait for reaper 7bc3b713: unexpected container status "created"`)

	testCases := []struct {
		name       string
		failures   int // starts that fail before one succeeds
		wantErr    bool
		wantStarts int
		wantSleeps []time.Duration
	}{
		{name: "starts first time: no retry", failures: 0, wantStarts: 1},
		{name: "a lost reaper race is retried after a growing wait", failures: 2, wantStarts: 3, wantSleeps: []time.Duration{2 * time.Second, 4 * time.Second}},
		{name: "still failing after every attempt: the last error", failures: 3, wantErr: true, wantStarts: 3, wantSleeps: []time.Duration{2 * time.Second, 4 * time.Second}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var slept []time.Duration
			orig := sleep
			sleep = func(d time.Duration) { slept = append(slept, d) }
			t.Cleanup(func() { sleep = orig })

			starts := 0
			_, err := startContainer("postgres", func() (testcontainers.Container, error) {
				starts++
				if starts <= tc.failures {
					return nil, reaperRace
				}
				return nil, nil
			})

			if tc.wantErr {
				require.ErrorIs(t, err, reaperRace)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.wantStarts, starts)
			require.Equal(t, tc.wantSleeps, slept)
		})
	}
}
