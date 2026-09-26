package migrations

import (
	"fmt"
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// baseTables are the tables of the first schema: one table per migration, 00001–00023.
var baseTables = []string{
	"organizations",
	"users",
	"houses",
	"premises",
	"registry_uploads",
	"owners",
	"owner_records",
	"memberships",
	"org_members",
	"decision_types",
	"templates",
	"template_items",
	"initiatives",
	"agenda_items",
	"poll_votes",
	"demands",
	"meetings",
	"ballots",
	"ballot_decisions",
	"gis_result_entries",
	"meeting_results",
	"audit_logs",
	"registry_correction_requests",
}

// Every migration after the base schema is named by its creation time:
// 20260926140000_poll_add_index.sql. Four people add migrations in parallel, and
// sequential numbers (00024, 00025, …) would collide between branches (docs/05).
var timestampName = regexp.MustCompile(`^(\d{14})_[a-z0-9_]+\.sql$`)

func TestMigrationFiles(t *testing.T) {
	files, err := fs.Glob(FS, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < len(baseTables) {
		t.Fatalf("got %d migration files, want at least %d", len(files), len(baseTables))
	}

	for _, f := range files {
		data, err := FS.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		sql := string(data)
		if strings.Count(sql, "-- +goose Up") != 1 || strings.Count(sql, "-- +goose Down") != 1 {
			t.Errorf("%s must contain exactly one goose Up and one goose Down section", f)
		}
	}

	checkBaseSchema(t, files[:len(baseTables)])

	// fs.Glob returns names sorted; with 14-digit versions the lexical order is the
	// version order, so a later migration can never get a lower number.
	prev := ""
	for _, f := range files[len(baseTables):] {
		m := timestampName.FindStringSubmatch(f)
		if m == nil {
			t.Errorf("%s: new migrations are named YYYYMMDDHHMMSS_<module>_<what>.sql", f)

			continue
		}
		if m[1] == prev {
			t.Errorf("%s: duplicate migration version %s", f, m[1])
		}
		prev = m[1]
	}
}

// checkBaseSchema keeps the rule of the first schema: one table per migration, in order.
func checkBaseSchema(t *testing.T, files []string) {
	t.Helper()

	for i, table := range baseTables {
		wantName := fmt.Sprintf("%05d_%s.sql", i+1, table)
		if files[i] != wantName {
			t.Errorf("migration %d is %q, want %q", i+1, files[i], wantName)

			continue
		}

		data, err := FS.ReadFile(files[i])
		if err != nil {
			t.Errorf("read %s: %v", files[i], err)

			continue
		}
		sql := string(data)
		if strings.Count(sql, "CREATE TABLE ") != 1 {
			t.Errorf("%s must create exactly one table", files[i])
		}
		if !strings.Contains(sql, "CREATE TABLE "+table+" (") {
			t.Errorf("%s does not create table %s", files[i], table)
		}
		if !strings.Contains(sql, "DROP TABLE "+table+";") {
			t.Errorf("%s does not drop table %s in Down", files[i], table)
		}
	}
}
