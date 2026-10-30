package logos

import (
	"database/sql"
	"testing"

	"ft/internal/db"
)

func testDB(t *testing.T) *sql.DB {
	d, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}
