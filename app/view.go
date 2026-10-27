package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

func (a *app) view(c *ui.Context) {
	a.applyTheme(c)
	a.reconcileSelection()
	c.OnShortcut(ui.Cmd, ui.KeyO, a.openFolder)
	c.OnShortcut(0, ui.KeyG, a.toggleGrid)
	c.OnShortcut(0, ui.KeyI, a.toggleInfo)
	c.OnShortcut(0, ui.KeyLeft, func() {
		if a.mode == viewSingle {
			a.selectRelative(-1)
		}
	})
	c.OnShortcut(0, ui.KeyRight, func() {
		if a.mode == viewSingle {
			a.selectRelative(1)
		}
	})
	c.OnShortcut(0, ui.KeyEscape, func() {
		if a.viewer.pressed {
			a.viewer.pressed, a.viewer.boxing = false, false
		} else if a.toolsOpen {
			a.setToolsOpen(false)
			a.filterOpen = false
		} else if a.mode == viewSingle {
			a.toggleGrid()
		} else {
			a.mode = viewSingle
		}
	})
	if a.mode == viewSingle {
		for _, mods := range []ui.Modifiers{0, ui.Cmd} {
			c.OnShortcut(mods, ui.KeyEqual, func() { a.viewer.zoomCenter(1.25) })
			c.OnShortcut(mods|ui.Shift, ui.KeyEqual, func() { a.viewer.zoomCenter(1.25) })
			c.OnShortcut(mods, ui.KeyMinus, func() { a.viewer.zoomCenter(.8) })
			c.OnShortcut(mods, ui.Key0, a.viewer.reset)
		}
	}
	t := c.Theme()
	root := ui.Column(c).Fill().Background(t.Background)
	root.Children(func() {
		a.compactToolbar(c)
		if a.toolsOpen {
			a.toolsBar(c)
		}
		if a.status != "" {
			ui.Row(c.Key("status")).Padding(6, 16).Gap(8).Children(func() {
				ui.Text(c, a.status).TextColor(t.Danger).Grow(1).SingleLine().Tooltip(a.status)
				ui.Button(c, "关闭").OnClick(func() { a.status = "" })
			})
		}
		ui.Box(c.Key("workspace")).Grow(1).MinHeight(0).Children(func() {
			if a.infoOpen {
				ui.SplitPercent(c, &a.mainPercent, func() { a.photoContent(c) }, func() { a.infoPanel(c) }).Fill()
			} else {
				a.photoContent(c)
			}
		})
	})
	if files := root.DroppedFiles(); len(files) > 0 {
		a.loadFolderAsync(files[0])
	}
}
func (a *app) applyTheme(c *ui.Context) {
	t := ui.DarkTheme()
	if a.themeMode == 1 || a.themeMode == 2 && !c.Theme().Dark {
		t = ui.LightTheme()
	}
	if t.Dark {
		t.Background = ui.Hex("#17191b")
		t.Surface = ui.Hex("#1b1d20")
		t.SurfaceHover = ui.Hex("#292d31")
		t.SurfacePressed = ui.Hex("#303732")
		t.Border = ui.Hex("#2b302f")
		t.Text = ui.Hex("#eceeeb")
		t.TextMuted = ui.Hex("#96a099")
		t.Accent = ui.Hex("#a9c7ae")
		t.AccentText = ui.Hex("#142119")
	} else {
		t.Background = ui.Hex("#f7f8f6")
		t.Surface = ui.Hex("#eef0ec")
		t.Border = ui.Hex("#dfe5de")
		t.Text = ui.Hex("#243029")
		t.TextMuted = ui.Hex("#6f7c72")
		t.Accent = ui.Hex("#486d52")
	}
	t.Spacing, t.Radius = 4, 5
	c.SetTheme(t)
}
func iconButton(c *ui.Context, key, label string, icon *ui.SVG, active bool) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c.Key(key)).Size(30, 28).Radius(5).Label(label).Tooltip(label)
	if active {
		b.TextColor(t.Accent).Background(t.Accent.Alpha(.10))
	} else {
		b.TextColor(t.TextMuted)
	}
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.Children(func() { ui.Icon(c, icon).Size(16, 16) })
	return b
}
func (a *app) compactToolbar(c *ui.Context) {
	t := c.Theme()
	titleBar := c.TitleBar()
	ui.Row(c.Key("toolbar")).Height(max(44, titleBar.Height)).Shrink(0).Padding(0, max(14, titleBar.Right+8), 0, max(14, titleBar.Left+12)).Gap(8).DragWindow().Background(t.Background).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		folder := ui.ButtonBase(c.Key("open")).Gap(8).Padding(5, 6).Radius(5).MaxWidth(230).Label("打开文件夹").Tooltip(a.dir + "  ⌘O")
		if folder.Hovered() {
			folder.Background(t.SurfaceHover)
		}
		folder.Children(func() {
			ui.Icon(c, iconFolder).Size(15, 15).TextColor(t.TextMuted)
			ui.Text(c, a.updateFolderLabel()).FontSize(12).SingleLine().MinWidth(0)
		})
		folder.OnClick(a.openFolder)
		if a.loading {
			ui.Spinner(c).Size(14, 14).Label("读取文件夹")
		}
		ui.Spacer(c)
		items := a.visiblePhotos()
		count := fmt.Sprintf("%d 张", len(items))
		if a.mode == viewSingle && len(items) > 0 {
			count = fmt.Sprintf("%d / %d", a.currentVisibleIndex(items)+1, len(items))
		}
		if a.dir != "" {
			ui.Text(c, count).FontSize(11).TextColor(t.TextMuted).SingleLine().MarginEnd(6)
		}
		if a.filter.active() {
			ui.Button(c, "筛选中").FontSize(11).TextColor(t.Accent).OnClick(func() { a.setToolsOpen(true); a.filterOpen = true })
		}
		if a.mode == viewSingle {
			iconButton(c, "grid", "网格", iconGrid, false).OnClick(a.toggleGrid)
		} else {
			iconButton(c, "thumb-smaller", "减小缩略图", iconMinus, false).Tooltip("减小缩略图 · Ctrl + 滚轮向下").Disabled(a.gridSize <= minGridSize).OnClick(func() { a.resizeThumbnails(a.gridSize - gridSizeStep) })
			iconButton(c, "thumb-larger", "增大缩略图", iconPlus, false).Tooltip("增大缩略图 · Ctrl + 滚轮向上").Disabled(a.gridSize >= maxGridSize).OnClick(func() { a.resizeThumbnails(a.gridSize + gridSizeStep) })
			iconButton(c, "thumb-reset", "复原缩略图", iconReset, false).Tooltip("复原缩略图大小").OnClick(a.resetThumbnails)
			iconButton(c, "single", "单图", iconSingle, false).OnClick(a.toggleGrid)
		}
		iconButton(c, "info", "信息", iconInfo, a.infoOpen).OnClick(a.toggleInfo)
		iconButton(c, "tools", "工具", iconTools, a.toolsOpen).OnClick(func() { a.setToolsOpen(!a.toolsOpen) })
	})
}
func (a *app) toolsBar(c *ui.Context) {
	t := c.Theme()
	ui.Column(c.Key("tools-panel")).Shrink(0).Background(t.Surface).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		ui.Row(c).Padding(6, 16).Gap(8).Wrap().Children(func() {
			direction := "↑"
			if a.sortDescending {
				direction = "↓"
			}
			ui.MenuButton(c.Key("sort"), a.sort.label()+" "+direction, func(m *ui.Menu) {
				for _, s := range sortModes {
					if m.Item(s.label()).Checked(a.sort == s).Chosen() {
						a.chooseSort(s)
					}
				}
				m.Separator()
				if m.Item("升序").Checked(!a.sortDescending).Chosen() {
					a.sortDescending = false
				}
				if m.Item("降序").Checked(a.sortDescending).Chosen() {
					a.sortDescending = true
				}
			}).Label("排序")
			a.formatMenu(c)
			ui.Button(c.Key("filter-toggle"), "筛选").OnClick(func() { a.filterOpen = !a.filterOpen })
			if a.filter.active() {
				ui.Button(c.Key("reset"), "清除筛选").OnClick(a.resetFilters)
			}
			ui.Spacer(c)
			if a.checkForUpdates != nil {
				ui.Button(c.Key("check-updates"), "检查更新…").OnClick(a.checkForUpdates)
			}
			ui.MenuButton(c.Key("theme"), "外观", func(m *ui.Menu) {
				for i, label := range []string{"深色", "浅色", "跟随系统"} {
					if m.Item(label).Checked(a.themeMode == i).Chosen() {
						a.themeMode = i
					}
				}
			})
		})
		if a.filterOpen {
			a.filterPanel(c)
		}
	})
}
func (a *app) formatMenu(c *ui.Context) {
	label := "格式"
	if a.filter.Ext != "" {
		label = strings.ToUpper(strings.TrimPrefix(a.filter.Ext, "."))
	}
	ui.MenuButton(c.Key("format"), label, func(m *ui.Menu) {
		if m.Item("全部格式").Checked(a.filter.Ext == "").Chosen() {
			a.filter.Ext = ""
		}
		for _, ext := range a.extensionOptions() {
			if m.Item(strings.ToUpper(strings.TrimPrefix(ext, "."))).Checked(a.filter.Ext == ext).Chosen() {
				a.filter.Ext = ext
			}
		}
	}).Label("格式筛选")
}
func (a *app) filterPanel(c *ui.Context) {
	t := c.Theme()
	ui.Column(c.Key("filters")).Padding(0, 16, 10).Gap(6).Children(func() {
		ui.Row(c).Gap(12).Wrap().Children(func() {
			ui.SearchField(c.Key("search"), &a.filter.Query).Label("搜索文件名").Placeholder("搜索文件名").Width(220)
			ui.Checkbox(c.Key("raw"), &a.filter.OnlyRaw, "仅 RAW")
			ui.Checkbox(c.Key("iso"), &a.filter.HasISO, "ISO 范围")
			ui.TextInput(c.Key("iso-min"), &a.isoMinText).Label("最小 ISO").Placeholder("最小").Width(70).Disabled(!a.filter.HasISO).OnChange(a.applyISORange)
			ui.Text(c, "—").TextColor(t.TextMuted)
			ui.TextInput(c.Key("iso-max"), &a.isoMaxText).Label("最大 ISO").Placeholder("最大").Width(70).Disabled(!a.filter.HasISO).OnChange(a.applyISORange)
		})
		if a.filterError != "" {
			ui.Text(c, a.filterError).FontSize(11).TextColor(t.Danger)
		}
	})
}
func (a *app) applyISORange() {
	minISO, minErr := strconv.Atoi(strings.TrimSpace(a.isoMinText))
	maxISO, maxErr := strconv.Atoi(strings.TrimSpace(a.isoMaxText))
	if strings.TrimSpace(a.isoMinText) == "" {
		minISO, minErr = 0, nil
	}
	if strings.TrimSpace(a.isoMaxText) == "" {
		maxISO, maxErr = 0, nil
	}
	if minErr != nil || maxErr != nil || minISO < 0 || maxISO < 0 || maxISO > 0 && minISO > maxISO {
		a.filterError = "请输入有效的 ISO 范围"
		return
	}
	a.filter.MinISO, a.filter.MaxISO, a.filterError = minISO, maxISO, ""
}
func (a *app) photoContent(c *ui.Context) {
	if len(a.visiblePhotos()) == 0 {
		a.emptyView(c)
		return
	}
	if a.mode == viewGrid {
		a.gridView(c)
	} else {
		a.imageStage(c)
	}
}
func (a *app) emptyView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c.Key("empty")).Fill().Center().Gap(14).Children(func() {
		title, detail := "打开一个文件夹开始浏览", "JPEG、PNG、WebP、TIFF 与相机 RAW"
		if a.loading {
			title, detail = "正在读取照片…", ""
		} else if a.dir != "" {
			title, detail = "文件夹里没有支持的图片", "可以选择另一个文件夹"
		}
		if a.filter.active() {
			title, detail = "没有符合条件的照片", "试试调整筛选条件"
		}
		ui.Icon(c, iconSingle).Size(32, 32).TextColor(t.TextMuted)
		ui.Text(c, title).FontSize(16)
		if detail != "" {
			ui.Text(c, detail).FontSize(12).TextColor(t.TextMuted)
		}
		if a.filter.active() {
			ui.Button(c, "清除筛选").OnClick(a.resetFilters)
		} else {
			ui.Button(c, "打开文件夹").OnClick(a.openFolder)
		}
	})
}
func (a *app) imageStage(c *ui.Context) {
	t := c.Theme()
	p, ok := a.selectedPhoto()
	if !ok {
		return
	}
	if a.viewer.path != p.Path {
		a.viewer = imageViewport{path: p.Path, viewport: a.viewer.viewport}
	}
	items := a.visiblePhotos()
	idx := a.currentVisibleIndex(items)
	baseTarget := a.singlePreviewEdge(c)
	previewTarget := a.zoomedPreviewEdge(p, baseTarget)
	a.preloadAdjacent(items, idx, baseTarget)
	ui.Column(c.Key("single-stage")).Fill().MinHeight(0).Background(ui.Hex("#111315")).Children(func() {
		ui.Box(c.Key("image-area")).Grow(1).MinHeight(0).Center().Children(func() {
			var placeholder *ui.Bitmap
			if p.Ext == ".jpg" || p.Ext == ".jpeg" || p.isRaw() {
				placeholder = a.cachedImage(p, false)
				if placeholder == nil {
					// Decode a small first frame before the window-sized preview on
					// direct opens. This avoids a blank stage and avoids decoding both
					// sizes at the same time when no grid was visited first.
					low, _, lowPending := a.image(p, false)
					placeholder = low
					if lowPending && low == nil {
						ui.Text(c, "正在读取图片…").TextColor(t.TextMuted).FontSize(12)
						return
					}
				}
			}
			bm, err, pending := a.image(p, true, previewTarget)
			switch {
			case bm != nil:
				a.photoCanvas(c, bm)
			case pending:
				if placeholder != nil {
					// The grid bitmap gives immediate feedback while the window-sized
					// preview is decoded in the urgent queue.
					a.photoCanvas(c, placeholder)
				} else {
					ui.Text(c, "正在读取图片…").TextColor(t.TextMuted).FontSize(12)
				}
			default:
				ui.Column(c).Center().Gap(8).Children(func() {
					ui.Text(c, "无法显示这张图片").TextColor(ui.Hex("#eceeeb"))
					if err != nil {
						ui.Text(c, err.Error()).MaxWidth(500).MaxLines(3).FontSize(12).TextColor(t.TextMuted)
					}
				})
			}
			if idx > 0 {
				a.edgeNavigation(c, "left-edge", "上一张", iconLeft, func() { a.selectRelative(-1) })
			}
			if idx >= 0 && idx < len(items)-1 {
				a.edgeNavigation(c, "right-edge", "下一张", iconRight, func() { a.selectRelative(1) })
			}
		})
		ui.Row(c.Key("navigation")).Height(44).Shrink(0).Padding(0, 12).AlignItems(ui.Center).Gap(8).Children(func() {
			iconButton(c, "previous", "上一张", iconLeft, false).Disabled(idx <= 0).OnClick(func() { a.selectRelative(-1) })
			iconButton(c, "next", "下一张", iconRight, false).Disabled(idx < 0 || idx >= len(items)-1).OnClick(func() { a.selectRelative(1) })
			ui.Text(c, p.Name).FontSize(11).TextColor(ui.Hex("#a0aaa3")).SingleLine().Grow(1).MinWidth(0)
			ready := a.viewer.width > 0
			iconButton(c, "zoom-out", "缩小", iconMinus, false).Disabled(!ready).OnClick(func() { a.viewer.zoomCenter(.8) })
			ui.Button(c.Key("zoom-fit-text"), "适合窗口").FontSize(11).MinWidth(68).Label("适合窗口").Tooltip("适合当前屏幕显示区域").Disabled(!ready).OnClick(a.viewer.reset)
			iconButton(c, "zoom-in", "放大", iconPlus, false).Disabled(!ready).OnClick(func() { a.viewer.zoomCenter(1.25) })
		})
	})
}

// The preview only needs to cover the current photo area at screen density.
// Bucketing keeps small window resizes from creating a new decode every frame.
func (a *app) singlePreviewEdge(c *ui.Context) int {
	w, h := a.viewer.viewport.W, a.viewer.viewport.H
	if w <= 0 || h <= 0 {
		w, h = c.Size()
		if a.infoOpen && a.mainPercent > 0 {
			w *= a.mainPercent / 100
		}
	}
	return imageEdgeBucket(int(max(w, h) * 2))
}

func (a *app) zoomedPreviewEdge(p photo, base int) int {
	if a.viewer.path != p.Path || a.viewer.width <= 0 || a.viewer.height <= 0 || a.viewer.scale <= 0 {
		return base
	}
	fit := a.viewer.fitScale()
	if fit <= 0 {
		return base
	}
	factor := a.viewer.currentScale() / fit
	if factor <= 1.25 {
		return base
	}
	maxSourceEdge := 0
	if p.Meta.Width > 0 && p.Meta.Height > 0 {
		maxSourceEdge = max(p.Meta.Width, p.Meta.Height)
	}
	target := int(float32(base) * factor)
	if maxSourceEdge > 0 {
		target = min(target, maxSourceEdge)
	}
	return imageEdgeBucket(target)
}

const singlePreloadRadius = 2

func (a *app) preloadAdjacent(items []photo, idx, edge int) {
	keep := make(map[string]bool, singlePreloadRadius*2+1)
	if idx >= 0 && idx < len(items) {
		for n := max(0, idx-singlePreloadRadius); n <= min(len(items)-1, idx+singlePreloadRadius); n++ {
			keep[items[n].Path] = true
		}
	}
	a.cancelStalePreloads(keep)
	if idx < 0 || idx >= len(items) {
		return
	}
	for n := max(0, idx-singlePreloadRadius); n <= min(len(items)-1, idx+singlePreloadRadius); n++ {
		if n != idx {
			a.preloadImage(items[n], edge)
		}
	}
}

func (a *app) edgeNavigation(c *ui.Context, key, label string, icon *ui.SVG, click func()) {
	edge := ui.Box(c.Key(key)).Absolute().Top(0).Bottom(0).Width(112).Center().Padding(0, 10).OnClick(click).Cursor(ui.CursorPointer)
	if c.Modifiers()&ui.Ctrl != 0 || a.viewer.pressed {
		edge.PassThrough()
	}
	if key == "right-edge" {
		edge = edge.Right(0)
	} else {
		edge = edge.Left(0)
	}
	if edge.Hovered() {
		edge.Background(ui.Hex("#ffffff").Alpha(.08))
		edge.Children(func() {
			ui.Icon(c, icon).Size(34, 34).TextColor(ui.Hex("#eceeeb"))
		})
	}
	edge.Label(label).Tooltip(label)
}
func (a *app) gridView(c *ui.Context) {
	t := c.Theme()
	items := a.visiblePhotos()
	grid := ui.GridView(c.Key("photo-grid"), &a.grid, len(items), a.gridSize, a.gridSize-10, func(i int) {
		p := items[i]
		ui.Column(c).Fill().Padding(5).Gap(7).Children(func() {
			ui.Box(c).Grow(1).MinHeight(0).Center().Background(ui.Hex("#111315")).Radius(3).Clip().Children(func() {
				bm, err, pending := a.image(p, false)
				if bm != nil {
					ui.Image(c, bm).Fit(ui.Contain).Fill().MinHeight(0)
				} else {
					label := p.typeLabel()
					if pending {
						label = "…"
					} else if err != nil {
						label += " · 无预览"
					}
					ui.Text(c, label).FontSize(11).TextColor(t.TextMuted)
				}
			})
			ui.Text(c, p.Name).FontSize(11).TextColor(t.TextMuted).SingleLine()
		})
	}).Fill().Padding(12)
	grid.HandleInput(a.gridInput)
	grid.OnChange(func() { a.selectVisible(a.visiblePhotos(), a.gridSelected) })
	grid.OnSubmit(func() { a.selectVisible(a.visiblePhotos(), a.gridSelected); a.viewer.reset(); a.mode = viewSingle })
	if a.focusGrid {
		a.grid.Focus(c)
		a.focusGrid = false
	}
}
func (a *app) infoPanel(c *ui.Context) {
	t := c.Theme()
	ui.Scroll(c.Key("inspector")).Fill().Background(t.Surface).Children(func() {
		ui.Column(c).Padding(22, 20).Gap(18).Selectable().Children(func() {
			ui.Text(c, "照片信息").FontSize(12).TextColor(t.TextMuted)
			p, ok := a.selectedPhoto()
			if !ok {
				ui.Text(c, "选择一张图片后显示").FontSize(12).TextColor(t.TextMuted)
				return
			}
			ui.Text(c, p.Name).FontSize(16).FontWeight(500).SingleLine().Tooltip(p.Name)
			ui.Row(c).Gap(12).Children(func() {
				exposureStat(c, "ISO", valueOrUnknown(strings.TrimPrefix(p.Meta.ISO, "ISO ")))
				exposureStat(c, "快门", valueOrUnknown(strings.TrimSuffix(p.Meta.Shutter, " s")))
				exposureStat(c, "光圈", valueOrUnknown(p.Meta.Aperture))
			})
			ui.Divider(c)
			a.infoRow(c, "相机", p.Meta.Camera)
			a.infoRow(c, "镜头", p.Meta.Lens)
			a.infoRow(c, "焦距", p.Meta.FocalLength)
			captured := ""
			if !p.Meta.Captured.IsZero() {
				captured = p.Meta.Captured.Format("2006-01-02 15:04:05")
			}
			a.infoRow(c, "拍摄时间", captured)
			ui.Divider(c)
			dimensions := ""
			if p.Meta.Width > 0 && p.Meta.Height > 0 {
				dimensions = fmt.Sprintf("%d × %d", p.Meta.Width, p.Meta.Height)
			}
			a.infoRow(c, "尺寸", dimensions)
			a.infoRow(c, "格式", p.typeLabel())
			a.infoRow(c, "大小", formatBytes(p.Size))
			a.infoRow(c, "修改时间", p.Modified.Format("2006-01-02 15:04"))
		})
	})
}
func exposureStat(c *ui.Context, label, value string) {
	t := c.Theme()
	ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(7).Children(func() {
		size := float32(17)
		if value == "未提供" {
			size = 12
		}
		ui.Text(c, value).FontSize(size).FontWeight(500).SingleLine().Tooltip(value)
		ui.Text(c, label).FontSize(10).TextColor(t.TextMuted)
	})
}
func (a *app) infoRow(c *ui.Context, label, value string) {
	t := c.Theme()
	ui.Row(c).Gap(10).AlignItems(ui.Start).Children(func() {
		ui.Text(c, label).FontSize(11).Width(52).Shrink(0).TextColor(t.TextMuted)
		ui.Text(c, valueOrUnknown(value)).FontSize(11).Grow(1).MinWidth(0).MaxLines(3).Tooltip(value)
	})
}
func valueOrUnknown(v string) string {
	if strings.TrimSpace(v) == "" {
		return "未提供"
	}
	return v
}
func formatBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1<<20 {
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
	if n < 1<<30 {
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	}
	return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
}
