package main

import (
	"math"

	"github.com/egoist/mygo/ui"
)

// Coordinates are local to the photo canvas. A zero scale follows the
// window; explicit scales and offsets change only the image, never the UI.
type imageViewport struct {
	handle                  ui.Handle
	path                    string
	width, height           float32
	viewport                ui.Rect
	scale                   float32
	pan                     ui.Point
	pressed, boxing, moved  bool
	button                  int
	start, end, originalPan ui.Point
}

func (v *imageViewport) reset() {
	v.scale, v.pan = 0, ui.Point{}
	v.pressed, v.boxing, v.moved = false, false, false
}

func (v *imageViewport) content() ui.Rect {
	return ui.Rect{X: 20, Y: 16, W: max(1, v.viewport.W-40), H: max(1, v.viewport.H-24)}
}

func (v *imageViewport) fitScale() float32 {
	r := v.content()
	if v.width <= 0 || v.height <= 0 {
		return 1
	}
	return min(r.W/v.width, r.H/v.height)
}

func (v *imageViewport) currentScale() float32 {
	if v.scale == 0 {
		return v.fitScale()
	}
	return v.scale
}

func (v *imageViewport) imageRect() ui.Rect {
	r, s := v.content(), v.currentScale()
	w, h := v.width*s, v.height*s
	return ui.Rect{X: r.X + (r.W-w)/2 + v.pan.X, Y: r.Y + (r.H-h)/2 + v.pan.Y, W: w, H: h}
}

func (v *imageViewport) clampPan() {
	r, s := v.content(), v.currentScale()
	x, y := max(0, (v.width*s-r.W)/2), max(0, (v.height*s-r.H)/2)
	v.pan.X, v.pan.Y = max(-x, min(x, v.pan.X)), max(-y, min(y, v.pan.Y))
}

func (v *imageViewport) zoomAt(factor float32, at ui.Point) {
	if v.width <= 0 || v.height <= 0 || factor <= 0 || math.IsNaN(float64(factor)) {
		return
	}
	old, s := v.imageRect(), v.currentScale()
	ix, iy := (at.X-old.X)/s, (at.Y-old.Y)/s
	v.scale = max(min(.02, v.fitScale()), min(32, s*factor))
	r := v.content()
	v.pan = ui.Point{X: at.X - r.X - r.W/2 - (ix-v.width/2)*v.scale, Y: at.Y - r.Y - r.H/2 - (iy-v.height/2)*v.scale}
	v.clampPan()
}

func (v *imageViewport) zoomCenter(factor float32) {
	r := v.content()
	v.zoomAt(factor, ui.Point{X: r.X + r.W/2, Y: r.Y + r.H/2})
}

func intersectRects(a, b ui.Rect) ui.Rect {
	x, y := max(a.X, b.X), max(a.Y, b.Y)
	return ui.Rect{X: x, Y: y, W: max(0, min(a.X+a.W, b.X+b.W)-x), H: max(0, min(a.Y+a.H, b.Y+b.H)-y)}
}

func (v *imageViewport) selection() ui.Rect {
	r := ui.Rect{X: min(v.start.X, v.end.X), Y: min(v.start.Y, v.end.Y), W: float32(math.Abs(float64(v.end.X - v.start.X))), H: float32(math.Abs(float64(v.end.Y - v.start.Y)))}
	return intersectRects(intersectRects(r, v.content()), v.imageRect())
}

func (v *imageViewport) fitSelection() {
	r, content, image := v.selection(), v.content(), v.imageRect()
	if r.W < 8 || r.H < 8 {
		return
	}
	s := v.currentScale()
	ix, iy := (r.X+r.W/2-image.X)/s, (r.Y+r.H/2-image.Y)/s
	v.scale = min(32, s*min(content.W/r.W, content.H/r.H))
	v.pan = ui.Point{X: (v.width/2 - ix) * v.scale, Y: (v.height/2 - iy) * v.scale}
	v.clampPan()
}

func (a *app) photoInput(ev ui.InputEvent) bool {
	v := &a.viewer
	point := ui.Point{X: ev.X, Y: ev.Y}
	switch ev.Kind {
	case ui.InputPointerDown:
		box := ev.Mods&ui.Ctrl != 0
		// macOS may report Control-click as a secondary press.
		if ev.Button != 0 && !(box && ev.Button == 1) {
			return false
		}
		v.pressed, v.boxing, v.moved, v.button = true, box, false, ev.Button
		v.start, v.end, v.originalPan = point, point, v.pan
		return true
	case ui.InputPointerMove:
		if !v.pressed {
			return false
		}
		v.end = point
		dx, dy := point.X-v.start.X, point.Y-v.start.Y
		v.moved = v.moved || dx*dx+dy*dy > 16
		if !v.boxing && v.moved {
			v.pan = ui.Point{X: v.originalPan.X + dx, Y: v.originalPan.Y + dy}
			v.clampPan()
		}
		return true
	case ui.InputPointerUp:
		if !v.pressed || ev.Button != v.button {
			return false
		}
		v.end = point
		if v.boxing && v.moved {
			v.fitSelection()
		}
		back := !v.boxing && !v.moved && ev.Clicks == 2
		v.pressed, v.boxing = false, false
		if back {
			a.toggleGrid()
		}
		return true
	case ui.InputZoom:
		v.zoomAt(ev.Scale, point)
		return true
	case ui.InputScroll:
		if v.pressed {
			return true
		}
		if ev.Precise && ev.Mods&ui.Ctrl == 0 {
			v.pan.X -= ev.DX
			v.pan.Y -= ev.DY
			v.clampPan()
		} else {
			v.zoomAt(float32(math.Exp(float64(max(-200, min(200, -ev.DY)))*.004)), point)
		}
		return true
	}
	return false
}

func (a *app) photoCanvas(c *ui.Context, bm *ui.Bitmap) {
	v := &a.viewer
	w, h := bm.Size()
	if v.width > 0 && v.height > 0 && v.scale > 0 && w > 0 && h > 0 {
		v.scale *= v.width / float32(w)
	}
	v.width, v.height = float32(w), float32(h)
	canvas := ui.Box(c.Key("photo-canvas")).Fill().MinWidth(0).MinHeight(0).Clip().Focusable().Bind(&v.handle).Label("图片预览").Description("双击返回网格；滚轮或捏合缩放；拖动平移；按住 Ctrl 拖框放大")
	bounds := canvas.Bounds()
	if bounds.W > 0 && bounds.H > 0 {
		v.viewport = ui.Rect{W: bounds.W, H: bounds.H}
		v.clampPan()
	} else {
		c.Invalidate()
	}
	canvas.HandleInput(func(ev ui.InputEvent) bool {
		// Handles remain valid between frames, including immediately after a
		// resize. Element handles themselves expire at the end of the build.
		bounds := v.handle.Bounds(c)
		if bounds.W > 0 && bounds.H > 0 {
			v.viewport = ui.Rect{W: bounds.W, H: bounds.H}
		}
		return a.photoInput(ev)
	})
	switch {
	case v.boxing || c.Modifiers()&ui.Ctrl != 0:
		canvas.Cursor(ui.CursorCrosshair)
	case v.pressed:
		canvas.Cursor(ui.CursorGrabbing)
	default:
		canvas.Cursor(ui.CursorGrab)
	}
	canvas.Draw(func(p *ui.Painter, r ui.Rect) {
		// Paint uses this frame's dimensions, including the first layout.
		view := *v
		view.viewport = ui.Rect{W: r.W, H: r.H}
		view.clampPan()
		image := view.imageRect()
		image.X += r.X
		image.Y += r.Y
		p.Clip(r, 0, func() {
			p.Image(bm, image, ui.Contain)
			if view.boxing && view.pressed {
				box := view.selection()
				box.X += r.X
				box.Y += r.Y
				p.Fill(box, ui.Hex("#a9c7ae").Alpha(.16), 0)
				p.Stroke(box, ui.Hex("#c5e6ca"), 0, 1)
			}
		})
	})
}
