package migrations

import "embed"

//go:embed sqlite/*.sql
var SQLiteMigrations embed.FS
