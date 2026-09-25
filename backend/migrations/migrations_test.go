package migrations

import (
	"fmt"
	"io/fs"
	"strings"
	"testing"
)

func TestOneTablePerMigration(t *testing.T) {
	tables := []string{
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

	files, err := fs.Glob(FS, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(tables) {
		t.Fatalf("got %d migration files, want %d", len(files), len(tables))
	}

	for i, table := range tables {
		wantName := fmt.Sprintf("%05d_%s.sql", i+1, table)
		if files[i] != wantName {
			t.Errorf("migration %d is %q, want %q", i+1, files[i], wantName)
			continue
		}

		data, readErr := FS.ReadFile(files[i])
		if readErr != nil {
			t.Errorf("read %s: %v", files[i], readErr)
			continue
		}
		sql := string(data)
		if strings.Count(sql, "-- +goose Up") != 1 || strings.Count(sql, "-- +goose Down") != 1 {
			t.Errorf("%s must contain exactly one goose Up and one goose Down section", files[i])
		}
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
