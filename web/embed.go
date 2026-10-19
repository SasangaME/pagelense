// Package web embeds the templates and static files.
package web

import "embed"

// FS holds the templates and static files.
//
//go:embed templates/*.html static/*
var FS embed.FS
