// Package goddiassets exposes the frontend and database migrations embedded
// into release binaries.
package goddiassets

import (
	"embed"
	"io/fs"
)

//go:embed all:web/dist migrations/*.sql
var embedded embed.FS

// Web returns the production frontend filesystem.
func Web() fs.FS {
	web, err := fs.Sub(embedded, "web/dist")
	if err != nil {
		panic(err)
	}
	return web
}

// Migrations returns the embedded goose migrations.
func Migrations() fs.FS {
	migrations, err := fs.Sub(embedded, "migrations")
	if err != nil {
		panic(err)
	}
	return migrations
}
