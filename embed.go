// Package assets embeds runtime resources into the binary at compile time.
// Migrations and default configuration remain embedded in the release.
package assets

import "embed"

//go:embed migrations
var MigrationsFS embed.FS

//go:embed configs
var ConfigsFS embed.FS
