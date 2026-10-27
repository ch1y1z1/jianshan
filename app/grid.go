package main

import (
	"math"

	"github.com/egoist/mygo/ui"
)

const (
	defaultGridSize float32 = 164
	minGridSize     float32 = 96
	maxGridSize     float32 = 320
	gridSizeStep    float32 = 24
)

func (a *app) resizeThumbnails(size float32) {
	if !math.IsNaN(float64(size)) {
		a.gridSize = max(minGridSize, min(maxGridSize, size))
	}
}

func (a *app) resetThumbnails() { a.gridSize = defaultGridSize }

func (a *app) gridInput(ev ui.InputEvent) bool {
	if ev.Mods&ui.Ctrl == 0 {
		return false
	}
	switch ev.Kind {
	case ui.InputScroll:
		if ev.DY == 0 {
			return false
		}
		if !ev.Momentum {
			a.resizeThumbnails(a.gridSize * float32(math.Exp(float64(max(-200, min(200, -ev.DY)))*.004)))
		}
		return true
	case ui.InputZoom:
		// Windows touchpads may report Control-scroll as a zoom event.
		if ev.Scale > 0 {
			a.resizeThumbnails(a.gridSize * ev.Scale)
		}
		return true
	}
	return false
}
