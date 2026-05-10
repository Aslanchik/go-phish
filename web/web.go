// Package web exposes the compiled React app as an embedded filesystem.
// The dist/ directory is produced by `npm run build` and is not committed.
// Run `cd web && npm run build` before `go build ./cmd/server/`.
package web

import "embed"

//go:embed dist
var StaticFS embed.FS
