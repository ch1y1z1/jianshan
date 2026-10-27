package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"math"
	"os"
	"time"

	"github.com/egoist/mygo/ui"
	"golang.org/x/image/draw"
)

const (
	thumbnailEdge   = 420
	previewEdge     = 4096
	thumbnailBudget = 64 << 20
	previewBudget   = 192 << 20
)

type imageKey struct {
	path  string
	large bool
}
type imageEntry struct {
	bitmap  *ui.Bitmap
	err     error
	pending bool
	used    uint64
	bytes   int64
	cancel  context.CancelFunc
}

type previewJob struct {
	key        imageKey
	photo      photo
	generation uint64
	ctx        context.Context
	entry      *imageEntry
}

// All cache state is owned by the UI thread. At most two decoding workers
// run concurrently; folder changes cancel outstanding work and stale results
// cannot enter the next folder's cache.
func (a *app) image(p photo, large bool) (*ui.Bitmap, error, bool) {
	key := imageKey{p.Path, large}
	a.cacheClock++
	if e := a.images[key]; e != nil {
		e.used = a.cacheClock
		return e.bitmap, e.err, e.pending
	}
	if a.win == nil {
		return nil, nil, false
	}
	ctx, cancel := context.WithCancel(a.ctx)
	e := &imageEntry{pending: true, used: a.cacheClock, cancel: cancel}
	job := previewJob{key: key, photo: p, generation: a.generation, ctx: ctx, entry: e}
	select {
	case a.jobs <- job:
		a.images[key] = e
	default:
		cancel()
		// The visible cells retry on the frame produced by the next completion.
	}
	return nil, nil, true
}
func (a *app) startWorkers() {
	for range 2 {
		go func() {
			for job := range a.jobs {
				if job.ctx.Err() != nil {
					continue
				}
				edge := thumbnailEdge
				if job.key.large {
					edge = previewEdge
				}
				img, err := decodePhoto(job.ctx, job.photo, edge)
				if job.ctx.Err() != nil {
					continue
				}
				// Constructing a bitmap doesn't touch a renderer. Ownership transfers
				// to the UI thread when the completion callback runs.
				var bitmap *ui.Bitmap
				var size int64
				if err == nil {
					bitmap = ui.NewBitmap(img)
					b := img.Bounds()
					size = int64(b.Dx()) * int64(b.Dy()) * 4
				}
				a.win.Update(func() {
					if a.generation != job.generation || a.images[job.key] != job.entry {
						return
					}
					job.entry.cancel()
					job.entry.cancel = nil
					job.entry.bitmap, job.entry.err, job.entry.pending, job.entry.bytes = bitmap, err, false, size
					a.trimImages(job.key.large, job.key)
				})
			}
		}()
	}
}
func (a *app) trimImages(large bool, keep imageKey) {
	budget := int64(thumbnailBudget)
	if large {
		budget = previewBudget
	}
	var total int64
	for k, e := range a.images {
		if k.large == large {
			total += e.bytes
		}
	}
	for total > budget {
		var oldest imageKey
		var found bool
		age := uint64(math.MaxUint64)
		for k, e := range a.images {
			if k.large == large && k != keep && !e.pending && e.bytes > 0 && e.used < age {
				oldest, age, found = k, e.used, true
			}
		}
		if !found {
			return
		}
		total -= a.images[oldest].bytes
		delete(a.images, oldest)
	}
}

func decodePhoto(ctx context.Context, p photo, edge int) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var data []byte
	var err error
	if p.isRaw() {
		data, err = rawPreview(ctx, p.Path)
	} else {
		// A single encoded file is read only when it is needed. Bound it so an
		// invalid multi-gigabyte file cannot exhaust the process before decoding.
		data, err = readEncoded(p.Path, 512<<20)
	}
	if err != nil {
		return nil, err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 120_000_000 {
		return nil, errors.New("图片尺寸超过当前版本的读取范围")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	// Embedded previews sometimes have their own orientation. Prefer it,
	// falling back to the camera's RAW orientation when the preview has none.
	orientation := parseJPEGMetadata(data, metadata{}).Orientation
	if orientation == 0 {
		orientation = p.Meta.Orientation
	}
	img = orientImage(img, orientation)
	b := img.Bounds()
	if max(b.Dx(), b.Dy()) <= edge {
		return img, nil
	}
	scale := float64(edge) / float64(max(b.Dx(), b.Dy()))
	dst := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(b.Dx())*scale)), max(1, int(float64(b.Dy())*scale))))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst, nil
}
func readEncoded(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if st.Size() > limit {
		return nil, fmt.Errorf("文件超过 %.0f MB", float64(limit)/(1<<20))
	}
	return readLimited(f, limit)
}

func orientImage(src image.Image, o int) image.Image {
	if o < 2 || o > 8 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := x, y
			switch o {
			case 2:
				dx = w - 1 - x
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dy = h - 1 - y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			dst.Set(dx, dy, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

// The timeout applies to external fallback decoders, which are cancellable
// processes. Go's built-in decoders are bounded by encoded and pixel limits.
func rawPreview(ctx context.Context, path string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if data, err := embeddedJPEG(ctx, path); err == nil {
		return data, nil
	}
	return platformRawPreview(ctx, path)
}
