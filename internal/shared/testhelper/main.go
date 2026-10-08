package testhelper

import (
	"context"
	"database/sql"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/utility"

	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
)

// RunWithDB is the body of a package's TestMain: it starts a migrated
// Postgres container (skipped under -short, where only unit tests run),
// hands it to setDB, runs the tests and exits.
//
//	var testDB *sql.DB
//
//	func TestMain(m *testing.M) {
//		testhelper.RunWithDB(m, func(db *sql.DB) { testDB = db })
//	}
func RunWithDB(m *testing.M, setDB func(db *sql.DB)) {
	gin.SetMode(gin.TestMode)

	// testing.Short() needs the flags parsed, which m.Run() would only do later.
	flag.Parse()

	if !testing.Short() {
		migrationPath := filepath.Join(utility.ProjectRoot(), "internal/platform/db/migration")
		db, cleanup := SetupTestDB(context.Background(), migrationPath)
		setDB(db)

		code := m.Run()
		cleanup()
		os.Exit(code)
	}

	os.Exit(m.Run())
}
