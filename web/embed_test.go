package web

import (
	"io/fs"
	"strings"
	"testing"
)

func TestDistIsEmbedded(t *testing.T) {
	page, err := fs.ReadFile(Dist, "dist/index.html")
	if err != nil {
		t.Fatalf("dist/index.html isn't embedded: %v (is the //go:embed line right? did npm run build make dist/?)", err)
	}
	if !strings.Contains(string(page), `<div id="root">`) {
		t.Errorf("dist/index.html doesn't look like the built React app")
	}
	assets, err := fs.ReadDir(Dist, "dist/assets")
	if err != nil || len(assets) == 0 {
		t.Errorf("dist/assets is missing or empty: %v", err)
	}
}
