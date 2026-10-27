package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestGridThumbnailControls(t *testing.T) {
	a := newApp()
	a.photos = []photo{testPhoto("photo.jpg", ".jpg", time.Now())}
	a.selected, a.mode = 0, viewGrid
	tt := ui.NewTester(a.view, 1000, 700)
	if err := tt.Click("增大缩略图"); err != nil {
		t.Fatal(err)
	}
	if a.gridSize <= defaultGridSize {
		t.Fatal("increase button did not resize thumbnails")
	}
	if err := tt.Click("减小缩略图"); err != nil {
		t.Fatal(err)
	}
	if a.gridSize != defaultGridSize {
		t.Fatal("decrease button did not resize thumbnails")
	}
	if !a.gridInput(ui.InputEvent{Kind: ui.InputScroll, Mods: ui.Ctrl, DY: -40}) {
		t.Fatal("Control-wheel was not handled")
	}
	tt.Frame()
	if a.gridSize <= defaultGridSize || a.selectedPath() != a.photos[0].Path || a.mode != viewGrid {
		t.Fatal("resize lost the grid selection")
	}
	if a.gridInput(ui.InputEvent{Kind: ui.InputScroll, DY: 40}) {
		t.Fatal("ordinary scrolling was intercepted")
	}
	if err := tt.Click("复原缩略图"); err != nil {
		t.Fatal(err)
	}
	if a.gridSize != defaultGridSize {
		t.Fatal("reset did not restore the default size")
	}
}
