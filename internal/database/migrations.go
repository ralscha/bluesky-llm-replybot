package database

import "embed"

//go:embed migration/*.sql
var MigrationFS embed.FS
