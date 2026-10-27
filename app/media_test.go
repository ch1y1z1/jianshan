package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilterRefreshKeepsReadyCacheAndCancelsOldQueue(t *testing.T) {
	a := newApp()
	a.photos = []photo{{Path: "keep.jpg", Name: "keep.jpg", Ext: ".jpg"}, {Path: "drop.png", Name: "drop.png", Ext: ".png"}}
	ready := &imageEntry{}
	ctx, cancel := context.WithCancel(a.ctx)
	pending := &imageEntry{pending: true, cancel: cancel}
	a.images[imageKey{path: "keep.jpg"}] = ready
	a.images[imageKey{path: "keep.jpg", large: true}] = pending
	a.images[imageKey{path: "drop.png"}] = &imageEntry{}
	a.jobs <- previewJob{ctx: ctx, entry: pending}
	a.filter.Ext = ".jpg"
	a.visiblePhotos()
	if ctx.Err() == nil || len(a.jobs) != 0 {
		t.Fatal("old decoding work was not canceled and drained")
	}
	if len(a.images) != 1 || a.images[imageKey{path: "keep.jpg"}] != ready {
		t.Fatal("filter discarded usable cache or kept excluded images")
	}
	a.sortDescending = true
	a.visiblePhotos()
	if a.images[imageKey{path: "keep.jpg"}] != ready {
		t.Fatal("sorting discarded reusable image data")
	}
}

func jpegFixture(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h)), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// Real EXIF byte layouts, including inline SHORT values and offset RATIONALs.
// The same fixture is generated in both TIFF byte orders.
func exifJPEG(t *testing.T, order binary.ByteOrder) []byte {
	t.Helper()
	type tag struct {
		id, kind uint16
		count    uint32
		data     []byte
	}
	short := func(n uint16) []byte { b := make([]byte, 2); order.PutUint16(b, n); return b }
	rational := func(n, d uint32) []byte {
		b := make([]byte, 8)
		order.PutUint32(b, n)
		order.PutUint32(b[4:], d)
		return b
	}
	tags := []tag{
		{0x010f, 2, 5, []byte("ACME\x00")},
		{0x0112, 3, 1, short(6)},
		{0x829a, 5, 1, rational(1, 125)},
		{0x829d, 5, 1, rational(28, 10)},
		{0x8827, 3, 1, short(400)},
		{0x9003, 2, 20, []byte("2026:10:27 12:34:56\x00")},
		{0x920a, 5, 1, rational(35, 1)},
	}
	b := make([]byte, 8+2+12*len(tags)+4)
	if order == binary.LittleEndian {
		copy(b, "II")
	} else {
		copy(b, "MM")
	}
	order.PutUint16(b[2:], 42)
	order.PutUint32(b[4:], 8)
	order.PutUint16(b[8:], uint16(len(tags)))
	for i, tag := range tags {
		at := 10 + i*12
		order.PutUint16(b[at:], tag.id)
		order.PutUint16(b[at+2:], tag.kind)
		order.PutUint32(b[at+4:], tag.count)
		if len(tag.data) <= 4 {
			copy(b[at+8:at+12], tag.data)
		} else {
			order.PutUint32(b[at+8:], uint32(len(b)))
			b = append(b, tag.data...)
		}
	}
	app1 := append([]byte("Exif\x00\x00"), b...)
	out := []byte{0xff, 0xd8, 0xff, 0xe1, byte((len(app1) + 2) >> 8), byte(len(app1) + 2)}
	out = append(out, app1...)
	return append(out, jpegFixture(t, 80, 40)[2:]...)
}
func TestExifByteOrdersAndOrientation(t *testing.T) {
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		data := exifJPEG(t, order)
		m := metadataFromBytes(data, ".jpg")
		if m.Camera != "ACME" || m.ISO != "ISO 400" || m.Shutter != "1/125 s" || m.Aperture != "f/2.8" || m.FocalLength != "35 mm" || m.Orientation != 6 || m.Captured.Year() != 2026 {
			t.Fatalf("EXIF %T = %+v", order, m)
		}
		path := filepath.Join(t.TempDir(), "oriented.jpg")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		img, err := decodePhoto(context.Background(), photo{Path: path, Ext: ".jpg", Meta: m}, 4096)
		if err != nil {
			t.Fatal(err)
		}
		if b := img.Bounds(); b.Dx() != 40 || b.Dy() != 80 {
			t.Fatalf("upright dimensions = %v", b)
		}
	}
}
func TestAllExifOrientations(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 3, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			src.SetRGBA(x, y, color.RGBA{R: uint8(1 + y*3 + x), A: 255})
		}
	}
	expected := map[int][]uint8{1: {1, 2, 3, 4, 5, 6}, 2: {3, 2, 1, 6, 5, 4}, 3: {6, 5, 4, 3, 2, 1}, 4: {4, 5, 6, 1, 2, 3}, 5: {1, 4, 2, 5, 3, 6}, 6: {4, 1, 5, 2, 6, 3}, 7: {6, 3, 5, 2, 4, 1}, 8: {3, 6, 2, 5, 1, 4}}
	for o, want := range expected {
		img := orientImage(src, o)
		b := img.Bounds()
		i := 0
		for y := 0; y < b.Dy(); y++ {
			for x := 0; x < b.Dx(); x++ {
				r, _, _, _ := img.At(x, y).RGBA()
				if uint8(r>>8) != want[i] {
					t.Fatalf("orientation %d pixel %d = %d want %d", o, i, r>>8, want[i])
				}
				i++
			}
		}
	}
}
func TestScanOnlyCurrentFolderWithoutDecoding(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.JPG", "b.png", "c.nef", ".hidden.jpg", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("not an image"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nested", "ignored.jpg"), jpegFixture(t, 32, 32), 0600); err != nil {
		t.Fatal(err)
	}
	items, err := scanFolder(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || items[0].Ext != ".jpg" || !items[2].isRaw() {
		t.Fatalf("scan = %+v", items)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := scanFolderContext(ctx, dir); err != context.Canceled {
		t.Fatalf("cancellation = %v", err)
	}
}
func TestEmbeddedRawSelectsLargestCompleteJPEG(t *testing.T) {
	// APP data contains a nested JPEG ending before the outer preview's EOI.
	small := jpegFixture(t, 16, 16)
	full := jpegFixture(t, 100, 60)
	app := append([]byte{0xff, 0xe1, byte((len(small) + 2) >> 8), byte(len(small) + 2)}, small...)
	outer := append([]byte{0xff, 0xd8}, app...)
	outer = append(outer, full[2:]...)
	raw := append([]byte("camera raw header"), small...)
	raw = append(raw, outer...)
	path := filepath.Join(t.TempDir(), "photo.nef")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	preview, err := embeddedJPEG(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(preview))
	if err != nil || cfg.Width != 100 || cfg.Height != 60 {
		t.Fatalf("selected preview = %+v %v", cfg, err)
	}
	img, err := decodePhoto(context.Background(), photo{Path: path, Ext: ".nef"}, 50)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 50 || b.Dy() != 30 {
		t.Fatalf("resized RAW = %v", b)
	}
}
func TestCorruptImageAndHeaderAreRecoverable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.jpg")
	if err := os.WriteFile(path, []byte{0xff, 0xd8, 0xff, 0xe1, 0xff, 0xff}, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := decodePhoto(context.Background(), photo{Path: path, Ext: ".jpg"}, 420); err == nil {
		t.Fatal("corrupt JPEG succeeded")
	}
	if _, err := embeddedJPEG(context.Background(), path); err == nil {
		t.Fatal("corrupt RAW succeeded")
	}
	// PNG dimensions still come from the file rather than demo data.
	var data bytes.Buffer
	_ = png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 31, 17)))
	m := metadataFromBytes(data.Bytes(), ".png")
	if m.Width != 31 || m.Height != 17 || m.ISO != "" {
		t.Fatalf("PNG metadata = %+v", m)
	}
}
func TestImageCacheEvictsOldEntries(t *testing.T) {
	a := newApp()
	oldest := imageKey{path: "old", large: true}
	newest := imageKey{path: "new", large: true}
	a.images[oldest] = &imageEntry{used: 1, bytes: 100 << 20}
	a.images[newest] = &imageEntry{used: 2, bytes: 100 << 20}
	a.trimImages(true, newest)
	if a.images[oldest] != nil || a.images[newest] == nil {
		t.Fatal("preview memory budget did not keep the newest image")
	}
}

// Optional acceptance check against a real camera file, supplied explicitly
// by the developer. It never reads the user's photo library automatically.
func TestRealRAWFixture(t *testing.T) {
	path := os.Getenv("JIANSHAN_RAW_FIXTURE")
	if path == "" {
		t.Skip("set JIANSHAN_RAW_FIXTURE to a camera RAW sample")
	}
	data, err := readHeader(path)
	if err != nil {
		t.Fatal(err)
	}
	p := photo{Path: path, Ext: strings.ToLower(filepath.Ext(path)), Meta: metadataFromBytes(data, strings.ToLower(filepath.Ext(path)))}
	img, err := decodePhoto(context.Background(), p, 420)
	if err != nil {
		t.Fatal(err)
	}
	if max(img.Bounds().Dx(), img.Bounds().Dy()) > 420 || img.Bounds().Dx() == 0 {
		t.Fatalf("RAW bounds = %v", img.Bounds())
	}
	t.Logf("RAW preview %v; metadata %+v", img.Bounds(), p.Meta)
}

func TestThumbnailDoesNotOverridePrimaryOrientation(t *testing.T) {
	b := make([]byte, 44)
	copy(b, "II")
	order := binary.LittleEndian
	order.PutUint16(b[2:], 42)
	order.PutUint32(b[4:], 8)
	// Primary IFD: orientation 6; next IFD points at an upright thumbnail.
	order.PutUint16(b[8:], 1)
	order.PutUint16(b[10:], 0x0112)
	order.PutUint16(b[12:], 3)
	order.PutUint32(b[14:], 1)
	order.PutUint16(b[18:], 6)
	order.PutUint32(b[22:], 26)
	order.PutUint16(b[26:], 1)
	order.PutUint16(b[28:], 0x0112)
	order.PutUint16(b[30:], 3)
	order.PutUint32(b[32:], 1)
	order.PutUint16(b[36:], 1)
	if m := parseTIFF(b, metadata{}); m.Orientation != 6 {
		t.Fatalf("primary orientation overwritten: %+v", m)
	}
}
