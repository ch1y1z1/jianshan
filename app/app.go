package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type app struct {
	win                             *mygo.Window
	checkForUpdates                 func()
	dir                             string
	photos                          []photo
	selected                        int
	mode                            viewMode
	infoOpen, toolsOpen, filterOpen bool
	mainPercent                     float32
	grid                            ui.GridState
	gridSelected                    int
	focusGrid                       bool
	viewer                          imageViewport

	filter                              filterState
	sort                                sortMode
	sortDescending                      bool
	visible                             []photo
	visibleValid                        bool
	lastFilter                          filterState
	lastSort                            sortMode
	lastDescending                      bool
	isoMinText, isoMaxText, filterError string

	loading, dialogOpen bool
	status              string
	themeMode           int // 0 dark, 1 light, 2 system
	generation          uint64
	ctx                 context.Context
	cancel              context.CancelFunc
	jobs                chan previewJob
	images              map[imageKey]*imageEntry
	cacheClock          uint64
}

func newApp() *app {
	ctx, cancel := context.WithCancel(context.Background())
	a := &app{selected: -1, gridSelected: -1, mode: viewSingle, infoOpen: true, mainPercent: 77, ctx: ctx, cancel: cancel, jobs: make(chan previewJob, 64), images: make(map[imageKey]*imageEntry)}
	a.grid.Selected = &a.gridSelected
	a.grid.Label = func(i int) string {
		items := a.visiblePhotos()
		if i < 0 || i >= len(items) {
			return ""
		}
		return items[i].Name
	}
	a.grid.Key = func(i int) any {
		items := a.visiblePhotos()
		if i < 0 || i >= len(items) {
			return i
		}
		return items[i].Path
	}
	return a
}

// Called on the UI thread. Switching folders cancels earlier scans and
// decodes; callbacks check the generation before changing visible state.
func (a *app) loadFolderAsync(path string) {
	if a.win == nil {
		return
	}
	a.cancel()
	a.ctx, a.cancel = context.WithCancel(context.Background())
	a.generation++
	generation, ctx, win := a.generation, a.ctx, a.win
	a.images = make(map[imageKey]*imageEntry)
	a.viewer = imageViewport{}
	a.loading, a.status = true, ""
	go func() {
		dir, selectedPath := path, ""
		st, err := os.Stat(path)
		if err == nil && !st.IsDir() {
			dir, selectedPath = filepath.Dir(path), path
		}
		var photos []photo
		if err == nil {
			photos, err = scanFolderContext(ctx, dir)
		}
		if ctx.Err() != nil {
			return
		}
		win.Update(func() {
			if a.generation != generation {
				return
			}
			a.loading = false
			if err != nil {
				a.status = fmt.Sprintf("无法打开文件夹：%v", err)
				return
			}
			a.dir, a.photos, a.selected = dir, photos, -1
			a.resetFilters()
			a.invalidatePhotos()
			a.grid = ui.GridState{Selected: &a.gridSelected, Label: a.grid.Label, Key: a.grid.Key}
			items := a.visiblePhotos()
			a.selectVisible(items, 0)
			for i := range items {
				if items[i].Path == selectedPath {
					a.selectVisible(items, i)
					break
				}
			}
			a.mode = viewSingle
			win.SetTitle("见山 · " + a.updateFolderLabel())
		})
	}()
}
func (a *app) openFolder() {
	if a.dialogOpen || a.win == nil {
		return
	}
	a.dialogOpen = true
	win, dir := a.win, a.dir
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: "打开照片文件夹", DefaultPath: dir, Directory: true})
		win.Update(func() {
			a.dialogOpen = false
			if err != nil {
				a.status = fmt.Sprintf("无法打开选择窗口：%v", err)
				return
			}
			if len(paths) > 0 {
				a.loadFolderAsync(paths[0])
			}
		})
	}()
}
func (a *app) toggleGrid() {
	a.viewer.reset()
	if a.mode == viewGrid {
		a.mode = viewSingle
		return
	}
	a.mode = viewGrid
	a.reconcileSelection()
	a.focusGrid = true
	a.grid.ScrollIntoView(max(a.gridSelected, 0))
}
func (a *app) resetFilters() {
	a.filter = filterState{}
	a.isoMinText, a.isoMaxText, a.filterError = "", "", ""
	a.filterOpen = false
}
func (a *app) chooseSort(s sortMode) { a.sort = s }
func (a *app) toggleSortDescending() { a.sortDescending = !a.sortDescending }
