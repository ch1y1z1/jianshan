package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPanelVisibilityPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings", "ui-settings.json")
	a := newApp()
	defer a.cancel()
	a.loadUISettings(path)
	if !a.infoOpen || a.toolsOpen {
		t.Fatal("missing settings changed initial defaults")
	}
	a.toggleInfo()
	a.setToolsOpen(true)

	b := newApp()
	defer b.cancel()
	b.loadUISettings(path)
	if b.infoOpen || !b.toolsOpen {
		t.Fatal("did not restore hidden info and open tools")
	}
	b.toggleInfo()
	b.setToolsOpen(false)
	a.loadUISettings(path)
	if !a.infoOpen || a.toolsOpen {
		t.Fatal("did not restore open info and hidden tools")
	}
	if err := os.WriteFile(path, []byte(`{"infoOpen":false,"toolsOpen":`), 0o600); err != nil {
		t.Fatal(err)
	}
	a.loadUISettings(path)
	if !a.infoOpen || a.toolsOpen {
		t.Fatal("damaged settings changed existing defaults")
	}
}
