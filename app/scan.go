package main

import (
	"bytes"
	"context"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

var previewExtensions = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".bmp": true, ".tif": true, ".tiff": true}
var rawExtensions = map[string]bool{".cr2": true, ".cr3": true, ".nef": true, ".nrw": true, ".arw": true, ".orf": true, ".rw2": true, ".raf": true, ".dng": true, ".pef": true, ".srw": true}

func supportedExtension(ext string) bool {
	ext = strings.ToLower(ext)
	return previewExtensions[ext] || rawExtensions[ext]
}

func scanFolder(dir string) ([]photo, error) { return scanFolderContext(context.Background(), dir) }
func scanFolderContext(ctx context.Context, dir string) ([]photo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	photos := make([]photo, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if !supportedExtension(ext) {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		p := photo{Path: filepath.Join(dir, entry.Name()), Name: entry.Name(), Ext: ext, Size: info.Size(), Modified: info.ModTime()}
		// Scanning reads only a bounded header. Pixel decoding is scheduled on
		// demand for the single image or currently visible grid cells.
		if data, err := readHeader(p.Path); err == nil {
			p.Meta = metadataFromBytes(data, ext)
		}
		photos = append(photos, p)
	}
	return photos, nil
}
func readHeader(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, 1<<20))
}
func metadataFromBytes(data []byte, ext string) metadata {
	m := metadata{}
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
		m.Width, m.Height = cfg.Width, cfg.Height
	}
	if ext == ".jpg" || ext == ".jpeg" {
		return parseJPEGMetadata(data, m)
	}
	if rawExtensions[ext] || ext == ".tif" || ext == ".tiff" {
		m = parseTIFF(data, m)
	}
	return m
}
