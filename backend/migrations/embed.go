// Package migrations holds the database schema as numbered SQL files.
// They're embedded into the binary, so the api carries its own schema
// and applies it on startup (see internal/db).
//
// Rules: never edit a migration that has been applied anywhere — add a new one.
// Every .up.sql has a matching .down.sql that undoes it.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
