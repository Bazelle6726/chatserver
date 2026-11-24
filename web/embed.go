// Package web holds the browser app (a React project, built with Vite) and
// embeds its built files, so the Go server can serve them with nothing
// next to the program.
//
// The React source is in src/. After changing it, rebuild with:
//
//	cd web
//	npm install     (once)
//	npm run build   (writes dist/)
//
// dist/ is committed, so the server builds without Node.js installed.
package web

import "embed"

// Dist holds the built app: dist/index.html, dist/assets/...
//
// The //go:embed line below makes the compiler fill Dist with every file
// under dist/, so they end up inside the server's .exe. Two details:
//   - no space after //: "// go:embed" is just a comment and does nothing
//   - embed can only reach files in this folder or below it. That's why this
//     small Go package lives here, next to dist/, and not in internal/api.
//
//go:embed dist
var Dist embed.FS
