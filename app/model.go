package main

import (
	"cmp"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

type viewMode uint8

const (
	viewSingle viewMode = iota
	viewGrid
)

type sortMode uint8

const (
	sortName sortMode = iota
	sortModified
	sortSize
	sortType
	sortCaptured
	sortPixels
	sortISO
	sortAperture
	sortShutter
	sortFocal
	sortCamera
)

var sortModes = []sortMode{sortName, sortModified, sortSize, sortType, sortCaptured, sortPixels, sortISO, sortAperture, sortShutter, sortFocal, sortCamera}

func (s sortMode) label() string {
	return []string{"文件名", "修改时间", "文件大小", "文件类型", "拍摄时间", "像素尺寸", "ISO", "光圈", "快门", "焦距", "相机"}[s]
}

type metadata struct {
	Camera, Lens, ISO, Shutter, Aperture, FocalLength string
	Captured                                          time.Time
	Width, Height                                     int
	Orientation                                       int
}

type photo struct {
	Path, Name, Ext string
	Size            int64
	Modified        time.Time
	Meta            metadata
}

func (p photo) isRaw() bool       { return rawExtensions[p.Ext] }
func (p photo) typeLabel() string { return strings.TrimPrefix(strings.ToUpper(p.Ext), ".") }

type filterState struct {
	Query, Ext      string
	MinISO, MaxISO  int
	HasISO, OnlyRaw bool
}

func (f filterState) active() bool {
	return strings.TrimSpace(f.Query) != "" || f.Ext != "" || f.HasISO || f.OnlyRaw
}
func (f filterState) matches(p photo) bool {
	q := strings.ToLower(strings.TrimSpace(f.Query))
	if q != "" && !strings.Contains(strings.ToLower(p.Name), q) {
		return false
	}
	if f.Ext != "" && p.Ext != f.Ext {
		return false
	}
	if f.OnlyRaw && !p.isRaw() {
		return false
	}
	if f.HasISO {
		n := parseISO(p.Meta.ISO)
		if n == 0 || n < f.MinISO || (f.MaxISO > 0 && n > f.MaxISO) {
			return false
		}
	}
	return true
}

// Results are rebuilt once when the folder, filter or ordering changes, not
// once per visible GridView cell. Selection stays anchored to the file path.
func (a *app) visiblePhotos() []photo {
	if a.visibleValid && a.lastFilter == a.filter && a.lastSort == a.sort && a.lastDescending == a.sortDescending {
		return a.visible
	}
	out := make([]photo, 0, len(a.photos))
	for _, p := range a.photos {
		if a.filter.matches(p) {
			out = append(out, p)
		}
	}
	a.sortPhotos(out)
	a.visible, a.visibleValid = out, true
	a.lastFilter, a.lastSort, a.lastDescending = a.filter, a.sort, a.sortDescending
	return out
}
func (a *app) invalidatePhotos() { a.visibleValid = false }

func (a *app) sortPhotos(items []photo) {
	slices.SortStableFunc(items, func(x, y photo) int {
		n := 0
		// Missing EXIF values stay at the end in both directions.
		switch a.sort {
		case sortCaptured:
			if x.Meta.Captured.IsZero() != y.Meta.Captured.IsZero() {
				if x.Meta.Captured.IsZero() {
					return 1
				}
				return -1
			}
			n = x.Meta.Captured.Compare(y.Meta.Captured)
		case sortModified:
			n = x.Modified.Compare(y.Modified)
		case sortSize:
			n = cmp.Compare(x.Size, y.Size)
		case sortType:
			n = strings.Compare(x.Ext, y.Ext)
		case sortCamera:
			if (x.Meta.Camera == "") != (y.Meta.Camera == "") {
				if x.Meta.Camera == "" {
					return 1
				}
				return -1
			}
			n = naturalCompare(x.Meta.Camera, y.Meta.Camera)
		case sortPixels, sortISO, sortAperture, sortShutter, sortFocal:
			xv, yv := sortValue(x, a.sort), sortValue(y, a.sort)
			if (xv == 0) != (yv == 0) {
				if xv == 0 {
					return 1
				}
				return -1
			}
			n = cmp.Compare(xv, yv)
		default:
			n = naturalCompare(x.Name, y.Name)
		}
		if n == 0 {
			n = naturalCompare(x.Name, y.Name)
		}
		if n == 0 {
			n = strings.Compare(x.Path, y.Path)
		}
		if a.sortDescending {
			return -n
		}
		return n
	})
}
func sortValue(p photo, s sortMode) float64 {
	switch s {
	case sortPixels:
		return float64(p.Meta.Width) * float64(p.Meta.Height)
	case sortISO:
		return float64(parseISO(p.Meta.ISO))
	case sortAperture:
		v, _ := strconv.ParseFloat(strings.TrimPrefix(p.Meta.Aperture, "f/"), 64)
		return v
	case sortFocal:
		v, _ := strconv.ParseFloat(strings.TrimSuffix(p.Meta.FocalLength, " mm"), 64)
		return v
	case sortShutter:
		v := strings.TrimSuffix(p.Meta.Shutter, " s")
		if strings.HasPrefix(v, "1/") {
			n, _ := strconv.ParseFloat(v[2:], 64)
			if n > 0 {
				return 1 / n
			}
			return 0
		}
		n, _ := strconv.ParseFloat(v, 64)
		return n
	}
	return 0
}

// Digit runs compare without conversion, so long camera sequence numbers
// cannot overflow: image2 precedes image10, with a stable spelling fallback.
func naturalCompare(x, y string) int {
	a, b := strings.ToLower(x), strings.ToLower(y)
	for i, j := 0, 0; i < len(a) || j < len(b); {
		if i == len(a) {
			return -1
		}
		if j == len(b) {
			return 1
		}
		if isDigit(a[i]) && isDigit(b[j]) {
			ai, bj := i, j
			for i < len(a) && isDigit(a[i]) {
				i++
			}
			for j < len(b) && isDigit(b[j]) {
				j++
			}
			ax, by := strings.TrimLeft(a[ai:i], "0"), strings.TrimLeft(b[bj:j], "0")
			if n := cmp.Compare(len(ax), len(by)); n != 0 {
				return n
			}
			if n := strings.Compare(ax, by); n != 0 {
				return n
			}
		} else {
			if a[i] != b[j] {
				return cmp.Compare(a[i], b[j])
			}
			i++
			j++
		}
	}
	return strings.Compare(x, y)
}
func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func (a *app) selectedPath() string {
	if a.selected >= 0 && a.selected < len(a.photos) {
		return a.photos[a.selected].Path
	}
	return ""
}
func (a *app) currentVisibleIndex(items []photo) int {
	path := a.selectedPath()
	for i, p := range items {
		if p.Path == path {
			return i
		}
	}
	return -1
}
func (a *app) selectVisible(items []photo, n int) {
	if len(items) == 0 {
		a.selected, a.gridSelected = -1, -1
		return
	}
	n = max(0, min(n, len(items)-1))
	path := items[n].Path
	for i := range a.photos {
		if a.photos[i].Path == path {
			a.selected, a.gridSelected = i, n
			return
		}
	}
}
func (a *app) reconcileSelection() {
	items := a.visiblePhotos()
	n := a.currentVisibleIndex(items)
	if n < 0 {
		a.selectVisible(items, 0)
	} else {
		a.gridSelected = n
	}
}
func (a *app) selectRelative(delta int) {
	items := a.visiblePhotos()
	n := a.currentVisibleIndex(items)
	if n < 0 {
		n = 0
	}
	a.selectVisible(items, n+delta)
}
func (a *app) selectedPhoto() (photo, bool) {
	if a.selected < 0 || a.selected >= len(a.photos) {
		return photo{}, false
	}
	return a.photos[a.selected], true
}
func (a *app) extensionOptions() []string {
	seen := map[string]bool{}
	for _, p := range a.photos {
		seen[p.Ext] = true
	}
	out := make([]string, 0, len(seen))
	for ext := range seen {
		out = append(out, ext)
	}
	slices.Sort(out)
	return out
}
func (a *app) updateFolderLabel() string {
	if a.dir == "" {
		return "打开文件夹"
	}
	return filepath.Base(a.dir)
}
func parseISO(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(strings.ToUpper(s), "ISO")))
	return n
}
