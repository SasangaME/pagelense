// Package web embeds the HTML templates and static assets.
package web

import "embed"

// FS holds templates/*.html and static/*, so the binary needs no files on disk.
//
//go:embed templates/*.html static/*
var FS embed.FS
