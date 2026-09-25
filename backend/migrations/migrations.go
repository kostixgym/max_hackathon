// Package migrations embeds SQL migrations into the binary: the service applies
// them on start, so a single `docker compose up` brings the schema up to date.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
