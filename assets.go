// Package zai2api — root package that embeds templates and static assets.
//
// Embedding assets into the binary makes it fully standalone: no need to
// ship templates/ or static/ alongside the binary. This fixes the
// "pattern matches no files: templates/*" panic that occurs when the
// binary is deployed to a directory without the assets (e.g., Windows
// deployment where the user copies only the .exe).
package zai2api

import "embed"

// Assets embeds all files under templates/ and static/ into the binary.
//
// The `all:` prefix is essential — without it, files starting with `.` or
// `_` (common for fonts and partial assets) are skipped by go:embed.
//
//go:embed all:templates all:static
var Assets embed.FS
