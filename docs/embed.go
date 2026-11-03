// Package docs embeds the product's operator manual.
package docs

import "embed"

// FS contains the manual served by the authenticated API.
//
//go:embed MANUAL.md
var FS embed.FS
