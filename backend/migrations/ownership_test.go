package migrations

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// tableOwners repeats «Модули и владение таблицами» from docs/04: the package of each
// module under internal/ and the tables it owns. A new table gets its owner here.
var tableOwners = map[string][]string{
	"registry": {
		"organizations", "houses", "premises", "owners", "owner_records",
		"registry_uploads", "registry_correction_requests",
	},
	"access":      {"users", "memberships", "org_members", "system_admins"},
	"rules":       {"decision_types", "templates", "template_items"},
	"initiatives": {"initiatives", "agenda_items", "initiative_questions"},
	"poll":        {"poll_votes"},
	"demand":      {"demands"},
	"meeting": {
		"meetings", "ballots", "ballot_decisions", "gis_result_entries", "meeting_results",
	},
	"audit":  {"audit_logs"},
	"notify": {"jobs", "bot_markers"},
}

var (
	createTable = regexp.MustCompile(`CREATE TABLE (?:IF NOT EXISTS )?([a-z_][a-z0-9_]*)`)
	// SQL keywords are upper case in our code, so English prose in comments does not match.
	tableRef = regexp.MustCompile(`\b(?:FROM|JOIN|INTO|UPDATE)\s+([a-z_][a-z0-9_]*)`)
)

// Principle 8 of docs/04: a module owns its tables and reads data of other modules
// only through their Go interfaces. Every table has exactly one owner, and the SQL
// in a module's code names only its own tables. Test files are not checked:
// fixtures fill any table.
func TestModulesOwnTheirTables(t *testing.T) {
	owner := map[string]string{}
	for module, tables := range tableOwners {
		for _, table := range tables {
			if other, ok := owner[table]; ok {
				t.Errorf("table %s is owned by both %s and %s", table, other, module)
			}
			owner[table] = module
		}
	}

	files, err := fs.Glob(FS, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	created := map[string]bool{}
	for _, f := range files {
		data, err := FS.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		for _, m := range createTable.FindAllStringSubmatch(string(data), -1) {
			created[m[1]] = true
			if owner[m[1]] == "" {
				t.Errorf("%s creates table %s without an owner module: add it to tableOwners and docs/04", f, m[1])
			}
		}
	}
	for table, module := range owner {
		if !created[table] {
			t.Errorf("tableOwners: table %s of %s is not created by any migration", table, module)
		}
	}

	const root = "../internal"
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		// internal/<module>/…; platform and bot own no tables.
		module, _, _ := strings.Cut(filepath.ToSlash(rel), "/")
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range tableRef.FindAllStringSubmatch(string(src), -1) {
			if table := m[1]; created[table] && owner[table] != module {
				t.Errorf("%s: SQL uses table %s of module %s; read it through that module's interface",
					rel, table, owner[table])
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
