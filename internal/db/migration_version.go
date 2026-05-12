//go:build !lite

package db

import (
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"
)

// LatestEmbeddedMigration returns the highest numeric prefix across the
// embedded migrations directory. Goose sorts by this prefix, so the value
// equals the version id stamped into goose_db_version once everything has
// been applied. Used by `breadbox doctor` and the headless bootstrap
// endpoint to detect "schema is behind embedded migrations".
//
// Returns 0 + an error if no migrations are present (which indicates the
// binary was built without the embed root).
func LatestEmbeddedMigration() (int64, error) {
	entries, err := fs.ReadDir(Migrations, "migrations")
	if err != nil {
		return 0, err
	}
	prefix := regexp.MustCompile(`^(\d+)_`)
	var versions []int64
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		m := prefix.FindStringSubmatch(e.Name())
		if len(m) != 2 {
			continue
		}
		var v int64
		if _, err := fmt.Sscanf(m[1], "%d", &v); err != nil {
			continue
		}
		versions = append(versions, v)
	}
	if len(versions) == 0 {
		return 0, fmt.Errorf("no migrations found")
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] < versions[j] })
	return versions[len(versions)-1], nil
}
