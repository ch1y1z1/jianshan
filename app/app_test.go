package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func testPhoto(name, ext string, modified time.Time) photo {
	return photo{
		Path:     "/tmp/" + name,
		Name:     name,
		Ext:      ext,
		Modified: modified,
		Size:     int64(len(name) * 100),
		Meta:     metadata{ISO: "ISO 400", Width: 32, Height: 24},
	}
}

func TestDefaultViewIsMinimalSingleImage(t *testing.T) {
	a := newApp()
	if a.mode != viewSingle || !a.infoOpen || a.toolsOpen || a.filterOpen {
		t.Fatalf("default state = mode %v info=%v tools=%v filter=%v", a.mode, a.infoOpen, a.toolsOpen, a.filterOpen)
	}
	tt := ui.NewTester(a.view, 900, 600)
	if !tt.HasText("打开一个文件夹开始浏览") || !tt.HasText("照片信息") {
		t.Fatalf("default UI texts = %v", tt.Texts())
	}
	if tt.HasText("排序：") || tt.HasText("ISO 范围") {
		t.Fatalf("collapsed controls leaked into default UI: %v", tt.Texts())
	}
}

func TestUpdateCheckIsAvailableInsideTools(t *testing.T) {
	a := newApp()
	checks := 0
	a.checkForUpdates = func() { checks++ }
	tt := ui.NewTester(a.view, 1100, 700)
	if tt.HasText("检查更新…") {
		t.Fatal("update entry leaked into the collapsed view")
	}
	if err := tt.Click("工具"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("检查更新…"); err != nil {
		t.Fatal(err)
	}
	if checks != 1 {
		t.Fatalf("update checks = %d", checks)
	}
}

func TestGridSelectionAndArrowNavigation(t *testing.T) {
	a := newApp()
	now := time.Now()
	a.photos = []photo{testPhoto("b.jpg", ".jpg", now), testPhoto("a.jpg", ".jpg", now.Add(-time.Hour))}
	a.selected = 0
	tt := ui.NewTester(a.view, 900, 600)
	if err := tt.Click("网格"); err != nil {
		t.Fatal(err)
	}
	if a.mode != viewGrid || !tt.HasText("单图") {
		t.Fatalf("grid toggle failed: mode=%v texts=%v", a.mode, tt.Texts())
	}
	// Grid order is name ascending, so the first cell is a.jpg.
	if err := tt.Click("a.jpg"); err != nil {
		t.Fatal(err)
	}
	if got := a.selectedPath(); got != "/tmp/a.jpg" {
		t.Fatalf("grid selection = %q", got)
	}
	if err := tt.Click("单图"); err != nil {
		t.Fatal(err)
	}
	if a.mode != viewSingle {
		t.Fatalf("single toggle failed: mode=%v", a.mode)
	}
	tt.Key(0, ui.KeyRight)
	if got := a.selectedPath(); got != "/tmp/b.jpg" {
		t.Fatalf("right arrow selected %q", got)
	}
	tt.Key(0, ui.KeyLeft)
	if got := a.selectedPath(); got != "/tmp/a.jpg" {
		t.Fatalf("left arrow selected %q", got)
	}
}

func TestFilteringAndSort(t *testing.T) {
	a := newApp()
	now := time.Now()
	a.photos = []photo{
		testPhoto("z.raw", ".arw", now.Add(3*time.Hour)),
		testPhoto("a.jpg", ".jpg", now.Add(time.Hour)),
		testPhoto("b.png", ".png", now),
	}
	a.filter.Ext = ".jpg"
	got := a.visiblePhotos()
	if len(got) != 1 || got[0].Name != "a.jpg" {
		t.Fatalf("extension filter = %+v", got)
	}
	a.filter = filterState{OnlyRaw: true}
	got = a.visiblePhotos()
	if len(got) != 1 || got[0].Name != "z.raw" {
		t.Fatalf("raw filter = %+v", got)
	}
	a.filter = filterState{}
	a.sort = sortModified
	a.sortDescending = true
	got = a.visiblePhotos()
	if got[0].Name != "z.raw" || got[2].Name != "b.png" {
		t.Fatalf("modified sort = %+v", got)
	}
}

func TestJPEGMetadataWithoutExifKeepsUnknowns(t *testing.T) {
	m := parseJPEGMetadata([]byte{0xff, 0xd8, 0xff, 0xd9}, metadata{Width: 100, Height: 80})
	if m.Width != 100 || m.Height != 80 || m.ISO != "" || m.Aperture != "" {
		t.Fatalf("metadata = %+v", m)
	}
}

func TestGridEnterOpensSelectionAndInfoFollows(t *testing.T) {
	a := newApp()
	now := time.Now()
	a.photos = []photo{testPhoto("first.jpg", ".jpg", now), testPhoto("second.png", ".png", now)}
	a.photos[1].Meta.ISO = "ISO 1600"
	a.selected = 0
	tt := ui.NewTester(a.view, 1000, 700)
	if err := tt.Click("网格"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyRight)
	if a.selectedPath() != "/tmp/second.png" || !tt.HasText("1600") {
		t.Fatalf("grid arrows/info selected %s: %v", a.selectedPath(), tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	if a.mode != viewSingle || !tt.HasText("1600") {
		t.Fatalf("Enter did not open selected photo")
	}
}
func TestFilterSelectionAndEmptyRecovery(t *testing.T) {
	a := newApp()
	now := time.Now()
	a.photos = []photo{testPhoto("first.jpg", ".jpg", now), testPhoto("second.png", ".png", now)}
	a.selected = 0
	tt := ui.NewTester(a.view, 1000, 700)
	a.filter.Ext = ".png"
	tt.Frame()
	if a.selectedPath() != "/tmp/second.png" {
		t.Fatalf("filter selected %q", a.selectedPath())
	}
	a.sortDescending = true
	tt.Frame()
	if a.selectedPath() != "/tmp/second.png" {
		t.Fatal("sorting changed selected file")
	}
	a.filter.Query = "no matches"
	tt.Frame()
	if a.selected != -1 || !tt.HasText("没有符合条件的照片") {
		t.Fatalf("empty state = %v", tt.Texts())
	}
	if err := tt.Click("清除筛选"); err != nil {
		t.Fatal(err)
	}
	if a.selected < 0 || a.filter.active() {
		t.Fatal("empty recovery failed")
	}
}
func TestMenusChangeRealOrderingAndSuffix(t *testing.T) {
	a := newApp()
	now := time.Now()
	a.photos = []photo{testPhoto("p10.jpg", ".jpg", now), testPhoto("p2.png", ".png", now), testPhoto("p1.jpg", ".jpg", now)}
	a.selected = 0
	tt := ui.NewTester(a.view, 1100, 700)
	if got := a.visiblePhotos(); got[0].Name != "p1.jpg" || got[1].Name != "p2.png" {
		t.Fatalf("natural sort %v", got)
	}
	if err := tt.Click("工具"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("格式筛选"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("PNG"); err != nil {
		t.Fatal(err)
	}
	if got := a.visiblePhotos(); len(got) != 1 || got[0].Name != "p2.png" {
		t.Fatal("suffix menu did not filter")
	}
	if err := tt.Click("排序"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("ISO"); err != nil {
		t.Fatal(err)
	}
	if a.sort != sortISO {
		t.Fatal("sort menu did not update")
	}
}
func TestMissingExifSortsLastBothDirections(t *testing.T) {
	a := newApp()
	a.photos = []photo{{Path: "a", Name: "a"}, {Path: "b", Name: "b", Meta: metadata{ISO: "ISO 400"}}, {Path: "c", Name: "c", Meta: metadata{ISO: "ISO 1600"}}}
	a.sort = sortISO
	for _, descending := range []bool{false, true} {
		a.sortDescending = descending
		got := a.visiblePhotos()
		if got[2].Path != "a" {
			t.Fatalf("unknown metadata not last: %v", got)
		}
	}
}

func TestTypingInFilterDoesNotInvokeViewShortcut(t *testing.T) {
	a := newApp()
	a.photos = []photo{testPhoto("g.jpg", ".jpg", time.Now())}
	a.selected = 0
	tt := ui.NewTester(a.view, 1100, 700)
	for _, label := range []string{"工具", "筛选", "搜索文件名"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	tt.TypeKey(0, ui.KeyG, "g")
	if a.mode != viewSingle || a.filter.Query != "g" {
		t.Fatalf("typing invoked shortcut: mode %v query %q", a.mode, a.filter.Query)
	}
}
