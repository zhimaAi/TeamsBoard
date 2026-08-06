//go:build !assets_web

package assets

import "embed"

// WebFS empty filesystem: used when the frontend is not built (dev mode uses Vite); registerWebUI falls back to the on-disk web/dist. web/dist.
var WebFS embed.FS
