package calendar

import (
	"context"
	"embed"
	"io/fs"
	"sort"
	"strings"

	"github.com/localitas/localitas-go"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func applyEmbeddedMigrations(ctx context.Context, c *client.Client, dbID string) error {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	for _, f := range files {
		sqlBytes, err := fs.ReadFile(migrationsFS, "migrations/"+f)
		if err != nil {
			return err
		}
		if _, err := c.ApplyDatabaseMigration(ctx, dbID, f, "", string(sqlBytes), ""); err != nil {
			return err
		}
	}
	return nil
}
