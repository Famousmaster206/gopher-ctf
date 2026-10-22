package static

import "embed"

// Files travels with the executable so deployments do not need the source tree.
//go:embed css images
var Files embed.FS
