package testhelper

import (
	"context"
	"fmt"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// StartRedis starts a redis:7-alpine container (with startContainer's
// retries) and returns its host:port and a function that stops it.
func StartRedis(ctx context.Context) (addr string, stop func(), err error) {
	container, err := startContainer("redis", func() (testcontainers.Container, error) {
		return testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image:        "redis:7-alpine",
				ExposedPorts: []string{"6379/tcp"},
				WaitingFor:   wait.ForLog("Ready to accept connections"),
			},
			Started: true,
		})
	})
	if err != nil {
		return "", nil, fmt.Errorf("testhelper.StartRedis(): %w", err)
	}
	stop = func() { _ = testcontainers.TerminateContainer(container) }
	host, err := container.Host(ctx)
	if err != nil {
		stop()
		return "", nil, fmt.Errorf("testhelper.StartRedis(): host: %w", err)
	}
	port, err := container.MappedPort(ctx, "6379/tcp")
	if err != nil {
		stop()
		return "", nil, fmt.Errorf("testhelper.StartRedis(): port: %w", err)
	}
	return host + ":" + port.Port(), stop, nil
}
