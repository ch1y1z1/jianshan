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
	"github.com/gen2brain/jpegn"
	"golang.org/x/image/draw"
)

const (
	thumbnailEdge   = 420
	previewEdge     = 4096
	maxPreviewEdge  = 8192
	thumbnailBudget = 64 << 20
	previewBudget   = 192 << 20
)

type imageKey struct {
	path  string
	large bool
	edge  int
}
type imageEntry struct {
	bitmap     *ui.Bitmap
	err        error
	pending    bool
	background bool
	full       bool
	used       uint64
	bytes      int64
	cancel     context.CancelFunc
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
func (a *app) image(p photo, large bool, requestedEdge ...int) (*ui.Bitmap, error, bool) {
	return a.requestImage(p, large, large, false, false, requestedEdge...)
}

func (a *app) preloadImage(p photo, requestedEdge int) {
	_, _, _ = a.requestImage(p, true, false, true, false, requestedEdge)
}

func (a *app) preloadFullImage(p photo, requestedEdge int) {
	_, _, _ = a.requestImage(p, true, true, true, true, requestedEdge)
}

func (a *app) requestImage(p photo, large, urgent, background, full bool, requestedEdge ...int) (*ui.Bitmap, error, bool) {
	edge := thumbnailEdge
	if large {
		edge = previewEdge
	}
	if len(requestedEdge) > 0 && requestedEdge[0] > 0 {
		edge = requestedEdge[0]
	}
	edge = imageEdgeBucket(edge)
	key := imageKey{path: p.Path, large: large, edge: edge}
	a.cacheClock++
	if e := a.images[key]; e != nil {
		if urgent && e.pending && e.background {
			if e.cancel != nil {
				e.cancel()
			}
			delete(a.images, key)
		} else {
			e.used = a.cacheClock
			return e.bitmap, e.err, e.pending
		}
	}
	if e := a.images[imageKey{path: p.Path, large: large}]; e != nil {
		if urgent && e.pending && e.background {
			if e.cancel != nil {
				e.cancel()
			}
			delete(a.images, imageKey{path: p.Path, large: large})
		} else {
			e.used = a.cacheClock
			return e.bitmap, e.err, e.pending
		}
	}
	bestEdge := int(^uint(0) >> 1)
	var ready *imageEntry
	for cachedKey, entry := range a.images {
		if cachedKey.path == p.Path && cachedKey.large == large && !entry.pending && entry.bitmap != nil && cachedKey.edge >= edge && cachedKey.edge < bestEdge {
			bestEdge, ready = cachedKey.edge, entry
		}
	}
	if ready != nil {
		ready.used = a.cacheClock
		return ready.bitmap, ready.err, false
	}
	if a.win == nil {
		return nil, nil, false
	}
	ctx, cancel := context.WithCancel(a.ctx)
	e := &imageEntry{pending: true, background: background, full: full, used: a.cacheClock, cancel: cancel}
	job := previewJob{key: key, photo: p, generation: a.generation, ctx: ctx, entry: e}
	a.images[key] = e
	select {
	case a.previewQueue(large, urgent, background) <- job:
	default:
		delete(a.images, key)
		cancel()
		// The visible cells retry on the frame produced by the next completion.
	}
	return nil, nil, true
}
func (a *app) previewQueue(large, urgent, background bool) chan previewJob {
	if urgent {
		return a.urgentJobs
	}
	if background {
		return a.preloadJobs
	}
	return a.jobs
}

func (a *app) cancelStalePreloads(keep map[string]bool, currentPath string) {
	for key, entry := range a.images {
		staleFull := entry.full && key.path != currentPath
		staleAdjacent := entry.background && !entry.full && !keep[key.path]
		if key.large && entry.pending && (staleFull || staleAdjacent) {
			if entry.cancel != nil {
				entry.cancel()
			}
			delete(a.images, key)
		}
	}
}

func imageEdgeBucket(edge int) int {
	edge = max(1, edge)
	if edge <= thumbnailEdge {
		return thumbnailEdge
	}
	return ((edge + 255) / 256) * 256
}

func (a *app) cachedImage(p photo, large bool) *ui.Bitmap {
	var best *imageEntry
	for key, entry := range a.images {
		if key.path == p.Path && key.large == large && !entry.pending && entry.bitmap != nil {
			if best == nil || key.edge > 0 && key.edge > imageEdgeBucket(thumbnailEdge) {
				best = entry
			}
		}
	}
	if best == nil {
		return nil
	}
	a.cacheClock++
	best.used = a.cacheClock
	return best.bitmap
}

func (a *app) cachedImageAtLeast(p photo, large bool, edge int) *ui.Bitmap {
	bestEdge := int(^uint(0) >> 1)
	var best *imageEntry
	for key, entry := range a.images {
		if key.path == p.Path && key.large == large && key.edge >= edge && !entry.pending && entry.bitmap != nil && key.edge < bestEdge {
			bestEdge, best = key.edge, entry
		}
	}
	if best == nil {
		return nil
	}
	a.cacheClock++
	best.used = a.cacheClock
	return best.bitmap
}

func (a *app) startWorkers() {
	for range 2 {
		go func() {
			for {
				// Poll urgent work first, then regular visible work, and only then
				// consume adjacent-photo preloads.
				select {
				case job, ok := <-a.urgentJobs:
					if !ok {
						return
					}
					a.processPreviewJob(job)
				default:
					select {
					case job, ok := <-a.jobs:
						if !ok {
							return
						}
						a.processPreviewJob(job)
					default:
						select {
						case job, ok := <-a.urgentJobs:
							if !ok {
								return
							}
							a.processPreviewJob(job)
						case job, ok := <-a.jobs:
							if !ok {
								return
							}
							a.processPreviewJob(job)
						case job, ok := <-a.preloadJobs:
							if !ok {
								return
							}
							a.processPreviewJob(job)
						}
					}
				}
			}
		}()
	}
}

func (a *app) processPreviewJob(job previewJob) {
	if job.ctx.Err() != nil {
		return
	}
	edge := job.key.edge
	if edge <= 0 {
		if job.key.large {
			edge = previewEdge
		} else {
			edge = thumbnailEdge
		}
	}
	img, err := decodePhoto(job.ctx, job.photo, edge)
	if job.ctx.Err() != nil {
		return
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
		if job.entry.cancel != nil {
			job.entry.cancel()
			job.entry.cancel = nil
		}
		job.entry.bitmap, job.entry.err, job.entry.pending, job.entry.bytes = bitmap, err, false, size
		a.trimImages(job.key.large, job.key)
	})
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
	} else if edge <= thumbnailEdge && (p.Ext == ".jpg" || p.Ext == ".jpeg") {
		// EXIF thumbnails are already close to grid size and avoid decoding the
		// camera's full-resolution JPEG. The complete file remains the fallback.
		data, err = readJPEGThumbnail(ctx, p.Path)
		if err != nil {
			data, err = readEncoded(p.Path, 512<<20)
		}
	} else {
		// A single encoded file is read only when it is needed. Bound it so an
		// invalid multi-gigabyte file cannot exhaust the process before decoding.
		data, err = readEncoded(p.Path, 512<<20)
	}
	if err != nil {
		return nil, err
	}
	jpegLike := p.isRaw() || p.Ext == ".jpg" || p.Ext == ".jpeg"
	if !jpegLike {
		cfg, _, decodeErr := image.DecodeConfig(bytes.NewReader(data))
		if decodeErr != nil {
			return nil, decodeErr
		}
		if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 120_000_000 {
			return nil, errors.New("图片尺寸超过当前版本的读取范围")
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var img image.Image
	if jpegLike {
		img, err = decodeJPEGAtSize(data, edge, edge <= thumbnailEdge)
	} else {
		img, _, err = image.Decode(bytes.NewReader(data))
	}
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

func decodeJPEGAtSize(data []byte, edge int, fast bool) (image.Image, error) {
	cfg, err := jpegn.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	denom := 1
	maxDimension := max(cfg.Width, cfg.Height)
	for denom < 8 && maxDimension/(denom*2) >= max(1, edge) {
		denom *= 2
	}
	method := jpegn.CatmullRom
	if fast {
		method = jpegn.NearestNeighbor
	}
	return jpegn.Decode(bytes.NewReader(data), &jpegn.Options{ScaleDenom: denom, UpsampleMethod: method})
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
