package main

import "github.com/egoist/mygo/ui"

func lineIcon(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`))
}

var (
	iconFolder = lineIcon(`<path d="M3 7V5a2 2 0 0 1 2-2h5l2 3h7a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z"/>`)
	iconGrid   = lineIcon(`<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/>`)
	iconSingle = lineIcon(`<rect x="3" y="4" width="18" height="16" rx="2"/><path d="m3 16 5-5 4 4 3-3 6 6"/><circle cx="15" cy="9" r="1"/>`)
	iconInfo   = lineIcon(`<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M15 4v16m3-11v6"/>`)
	iconTools  = lineIcon(`<path d="M4 6h16M4 12h16M4 18h16"/><circle cx="9" cy="6" r="2"/><circle cx="16" cy="12" r="2"/><circle cx="9" cy="18" r="2"/>`)
	iconLeft   = lineIcon(`<path d="m15 5-7 7 7 7"/>`)
	iconRight  = lineIcon(`<path d="m9 5 7 7-7 7"/>`)
	iconMinus  = lineIcon(`<path d="M5 12h14"/>`)
	iconPlus   = lineIcon(`<path d="M5 12h14m-7-7v14"/>`)
	iconFit    = lineIcon(`<path d="M8 3H3v5m13-5h5v5M3 16v5h5m13-5v5h-5"/>`)
)
