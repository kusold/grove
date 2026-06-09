package db

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kusold/grove/internal/integrationtest"
)

func TestOpenConnectsToPostgres18(t *testing.T) {
	databaseURL := integrationtest.Postgres18(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	database, err := Open(ctx, Config{
		URL:            databaseURL,
		MaxConns:       4,
		MinConns:       0,
		ConnectTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Open() returned unexpected error: %v", err)
	}
	t.Cleanup(database.Close)

	if err := database.Ping(ctx); err != nil {
		t.Fatalf("Ping() returned unexpected error: %v", err)
	}

	var versionNum string
	if err := database.Pool().QueryRow(ctx, "show server_version_num").Scan(&versionNum); err != nil {
		t.Fatalf("query server version: %v", err)
	}
	if !strings.HasPrefix(versionNum, "18") {
		t.Fatalf("server_version_num = %q, want Postgres 18", versionNum)
	}
}
