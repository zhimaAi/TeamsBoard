// Package assets embeds runtime resources into the binary at compile time, making the release a single executable,/// no longer depending on sibling folders such as migrations/, skills/, configs/, and web/dist next to the executable.exec// Resources live under the source tree root:s//   - migrations/  database migration SQL (always embedded)Q//   - skills/      built-in Skills (always embedded)i//   - configs/     default config files (always embedded) //   - web/dist     frontend build output (embedded only with -tags assets_web; dev mode is served by Vite, see embed_web_off.go)ite,// At runtime they are read via assets.MigrationsFS / SkillsFS / ConfigsFS / WebFS, with no external files needed.FS, with no external files needed.
package assets

import "embed"

//go:embed migrations
var MigrationsFS embed.FS

//go:embed skills
var SkillsFS embed.FS

//go:embed configs
var ConfigsFS embed.FS
