package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/plugins/updater/native"
)

func (a *app) useUpdater() {
	// Background checks default to once a day. Downloading and installing
	// remain a user choice in the native update window.
	mygo.Use(native.New(updater.Options{Language: "zh-Hans"}))
	a.checkForUpdates = updater.CheckForUpdates
}

func (a *app) setApplicationMenu() {
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Label: "见山", Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleAbout},
			updater.MenuItem(),
			mygo.Separator(),
			{Role: mygo.RoleHide},
			{Role: mygo.RoleHideOthers},
			{Role: mygo.RoleUnhide},
			mygo.Separator(),
			{Role: mygo.RoleQuit},
		}},
		{Label: "文件", Submenu: []*mygo.MenuItem{
			{Label: "打开文件夹…", Accelerator: "CmdOrCtrl+O", Click: func(*mygo.MenuItem, *mygo.Window) { a.openFolder() }},
			{Role: mygo.RoleClose},
		}},
		{Role: mygo.RoleEditMenu},
		{Role: mygo.RoleWindowMenu},
	}))
}
