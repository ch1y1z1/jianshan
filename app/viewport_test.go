package main

import (
	"image"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestPhotoViewportInteractions(t *testing.T) {
	a := newApp()
	a.photos = []photo{testPhoto("photo.jpg", ".jpg", time.Now())}
	a.selected = 0
	a.images[imageKey{a.photos[0].Path, true}] = &imageEntry{bitmap: ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 1200, 800)))}
	tt := ui.NewTester(a.view, 1000, 700)
	tt.Frame()
	r, ok := tt.Find("图片预览")
	if !ok {
		t.Fatal("photo canvas is absent")
	}
	fit := a.viewer.currentScale()
	tt.Wheel(r.X+r.W/2, r.Y+r.H/2, 0, -80)
	if a.viewer.currentScale() <= fit {
		t.Fatal("wheel did not zoom the photo")
	}
	if err := tt.Click("适应窗口"); err != nil {
		t.Fatal(err)
	}
	if a.viewer.scale != 0 {
		t.Fatal("fit did not restore automatic scaling")
	}

	// Control-drag zooms to the selected part, retaining its center.
	imageRect := a.viewer.imageRect()
	x, y := imageRect.X+imageRect.W/4, imageRect.Y+imageRect.H/4
	for _, ev := range []ui.InputEvent{
		{Kind: ui.InputPointerDown, X: x, Y: y, Mods: ui.Ctrl},
		{Kind: ui.InputPointerMove, X: x + imageRect.W/3, Y: y + imageRect.H/3, Mods: ui.Ctrl},
		{Kind: ui.InputPointerUp, X: x + imageRect.W/3, Y: y + imageRect.H/3, Mods: ui.Ctrl},
	} {
		if !a.photoInput(ev) {
			t.Fatal("box gesture was not handled")
		}
	}
	if a.viewer.currentScale() <= fit || a.viewer.boxing || a.viewer.pressed {
		t.Fatal("box zoom did not finish")
	}
	tt.Key(0, ui.Key0)
	if a.viewer.scale != 0 || a.viewer.pan != (ui.Point{}) {
		t.Fatal("0 did not reset the view")
	}

	for range 2 {
		if err := tt.Click("图片预览"); err != nil {
			t.Fatal(err)
		}
	}
	if a.mode != viewGrid {
		t.Fatal("double click did not return to the grid")
	}
	for range 2 {
		if err := tt.Click("photo.jpg"); err != nil {
			t.Fatal(err)
		}
	}
	if a.mode != viewSingle {
		t.Fatal("grid double click did not reopen the photo")
	}
}
