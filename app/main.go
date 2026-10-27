package main

import (
	_ "embed"
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/locales/zh-Hans"
	"github.com/egoist/mygo/ui"
)

//go:embed mygo.json
var applicationConfig []byte

func main() {
	a := newApp()
	// Packaged builds get their identity from MyGo. Development runs read
	// the same configuration, so a version bump has a single source.
	if !mygo.App.IsPackaged() {
		var info struct {
			Name    string
			Version string
		}
		if err := json.Unmarshal(applicationConfig, &info); err != nil {
			log.Fatal(err)
		}
		mygo.App.SetName(info.Name)
		mygo.App.SetVersion(info.Version)
	}
	mygo.App.SetLocale("zh-Hans")
	a.useUpdater()
	mygo.App.WhenReady(func() {
		a.setApplicationMenu()
		a.win = mygo.NewWindow(mygo.WindowOptions{Title: "见山", Width: 1180, Height: 760, MinWidth: 760, MinHeight: 520, TitleBarStyle: mygo.TitleBarHiddenInset, TitleBarHeight: 44, BackgroundColor: "#17191b", StateKey: "jianshan.main", Content: ui.View(a.view)})
		a.startWorkers()
		a.win.OnClosed(func() { a.cancel(); close(a.jobs) })
		if len(os.Args) > 1 {
			if path, err := filepath.Abs(os.Args[1]); err == nil {
				a.loadFolderAsync(path)
			}
		}
	})
	mygo.App.OnOpenFile(func(path string) {
		if a.win != nil {
			a.loadFolderAsync(path)
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
