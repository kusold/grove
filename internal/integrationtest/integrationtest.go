// Package integrationtest provides helpers for Grove integration tests.
package integrationtest

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

const (
	postgres18Image  = "postgres:18"
	postgresDB       = "grove_test"
	postgresUser     = "grove"
	postgresPassword = "grove"
)

// Require skips the calling test when Go's short test mode is enabled.
func Require(t testing.TB) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
}

// Postgres18 starts a Postgres 18 container and returns a connection string.
func Postgres18(t testing.TB) string {
	t.Helper()
	Require(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	container, err := tcpostgres.Run(ctx,
		postgres18Image,
		tcpostgres.WithDatabase(postgresDB),
		tcpostgres.WithUsername(postgresUser),
		tcpostgres.WithPassword(postgresPassword),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start Postgres 18 container: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Errorf("terminate Postgres 18 container: %v", err)
		}
	})

	databaseURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("build Postgres 18 connection string: %v", err)
	}
	return databaseURL
}
