package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

type uiSettings struct {
	InfoOpen  bool `json:"infoOpen"`
	ToolsOpen bool `json:"toolsOpen"`
}

func (a *app) loadUISettings(path string) {
	a.uiSettingsPath = path
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		log.Printf("无法读取界面设置：%v", err)
		return
	}
	settings := uiSettings{InfoOpen: a.infoOpen, ToolsOpen: a.toolsOpen}
	if err := json.Unmarshal(data, &settings); err != nil {
		log.Printf("无法解析界面设置：%v", err)
		return
	}
	a.infoOpen, a.toolsOpen = settings.InfoOpen, settings.ToolsOpen
}

func (a *app) saveUISettings() {
	if a.uiSettingsPath == "" {
		return
	}
	data, err := json.MarshalIndent(uiSettings{InfoOpen: a.infoOpen, ToolsOpen: a.toolsOpen}, "", "  ")
	if err == nil {
		err = os.MkdirAll(filepath.Dir(a.uiSettingsPath), 0o755)
	}
	// Replace the file only after writing succeeds, preserving the last saved state.
	tmp := a.uiSettingsPath + ".tmp"
	defer os.Remove(tmp)
	if err == nil {
		err = os.WriteFile(tmp, data, 0o600)
	}
	if err == nil {
		err = os.Rename(tmp, a.uiSettingsPath)
	}
	if err != nil {
		log.Printf("无法保存界面设置：%v", err)
	}
}

func (a *app) toggleInfo() {
	a.infoOpen = !a.infoOpen
	a.saveUISettings()
}

func (a *app) setToolsOpen(open bool) {
	if a.toolsOpen != open {
		a.toolsOpen = open
		a.saveUISettings()
	}
}
