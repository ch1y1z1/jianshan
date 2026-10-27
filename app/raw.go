package main

import (
	"bytes"
	"context"
	"errors"
	"image/jpeg"
	"io"
	"os"
)

func readLimited(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("图像数据超出读取范围")
	}
	return b, nil
}

// Many camera RAW files carry a normal JPEG made by the camera. Choose the
// largest complete embedded JPEG, not the first EXIF thumbnail. This works
// without native libraries on every platform; files with no usable preview
// fall back to the platform decoder. No source file is modified.
func embeddedJPEG(ctx context.Context, path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 64<<20))
	if err != nil {
		return nil, err
	}
	var best []byte
	var area int64
	for at := 0; at < len(data)-2; {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		offset := bytes.Index(data[at:], []byte{0xff, 0xd8})
		if offset < 0 {
			break
		}
		at += offset
		n := jpegLength(data[at:])
		if n > 0 {
			candidate := data[at : at+n]
			if cfg, err := jpeg.DecodeConfig(bytes.NewReader(candidate)); err == nil && cfg.Width >= 16 && cfg.Height >= 16 {
				pixels := int64(cfg.Width) * int64(cfg.Height)
				if pixels > area && pixels <= 120_000_000 {
					best, area = candidate, pixels
				}
			}
			at += n
		} else {
			at += 2
		}
	}
	if len(best) == 0 {
		return nil, errors.New("未找到可用的内嵌预览")
	}
	return bytes.Clone(best), nil
}

// Walk JPEG segments, skipping APP payloads and stuffed entropy bytes. An
// EOI inside an EXIF thumbnail must not end its containing full-size JPEG.
func jpegLength(b []byte) int {
	if len(b) < 2 || b[0] != 0xff || b[1] != 0xd8 {
		return 0
	}
	for i := 2; i < len(b); {
		if b[i] != 0xff {
			return 0
		}
		for i < len(b) && b[i] == 0xff {
			i++
		}
		if i == len(b) {
			return 0
		}
		marker := b[i]
		i++
		if marker == 0xd9 {
			return i
		}
		if marker == 0xd8 || marker == 0x00 {
			return 0
		}
		if marker == 1 || marker >= 0xd0 && marker <= 0xd7 {
			continue
		}
		if i+2 > len(b) {
			return 0
		}
		length := int(b[i])<<8 | int(b[i+1])
		if length < 2 || length > len(b)-i {
			return 0
		}
		i += length
		if marker != 0xda {
			continue
		}
		for i < len(b) {
			p := bytes.IndexByte(b[i:], 0xff)
			if p < 0 {
				return 0
			}
			i += p
			j := i + 1
			for j < len(b) && b[j] == 0xff {
				j++
			}
			if j == len(b) {
				return 0
			}
			if b[j] == 0 || b[j] >= 0xd0 && b[j] <= 0xd7 {
				i = j + 1
				continue
			}
			break
		}
	}
	return 0
}
