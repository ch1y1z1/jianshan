//go:build darwin

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// sips is macOS's ImageIO command. It provides a real system RAW fallback
// for supported cameras, with no cgo or runtime package installation.
func platformRawPreview(ctx context.Context, path string) ([]byte, error) {
	dir, err := os.MkdirTemp("", "jianshan-raw-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	output := filepath.Join(dir, "preview.jpg")
	cmd := exec.CommandContext(ctx, "/usr/bin/sips", "-s", "format", "jpeg", "--resampleHeightWidthMax", "4096", path, "--out", output)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("此 RAW 文件没有可用预览，系统也无法读取：%w", err)
	}
	return readEncoded(output, 64<<20)
}
