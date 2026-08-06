//go:build assets_web

package assets

import "embed"

// "all:" is required here: Vite emits helper chunks whose names start with "_".
// Without it, embed silently omits those files and the packaged UI fails at runtime.
//
//go:embed all:web/dist
var WebFS embed.FS
