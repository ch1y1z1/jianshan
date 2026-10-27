package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"os"
)

// readJPEGThumbnail reads the JPEG thumbnail stored in an EXIF IFD1. It only
// reads an early prefix because EXIF metadata and its thumbnail precede the
// main image scan in normal camera JPEGs.
func readJPEGThumbnail(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	// Keep this a prefix read: a large main JPEG may be much bigger than the
	// prefix while its EXIF thumbnail is still near the beginning.
	data, err := io.ReadAll(io.LimitReader(f, 8<<20))
	if err != nil {
		return nil, err
	}
	return jpegThumbnailFromData(data)
}

func jpegThumbnailFromData(data []byte) ([]byte, error) {
	if len(data) < 4 || data[0] != 0xff || data[1] != 0xd8 {
		return nil, errors.New("不是 JPEG 文件")
	}
	for i := 2; i+1 < len(data); {
		if data[i] != 0xff {
			i++
			continue
		}
		for i < len(data) && data[i] == 0xff {
			i++
		}
		if i >= len(data) {
			break
		}
		marker := data[i]
		i++
		if marker == 0xda || marker == 0xd9 {
			break
		}
		if marker >= 0xd0 && marker <= 0xd7 || marker == 0x01 {
			continue
		}
		if i+2 > len(data) {
			break
		}
		length := int(data[i])<<8 | int(data[i+1])
		if length < 2 || i+length > len(data) {
			break
		}
		segment := data[i+2 : i+length]
		if marker == 0xe1 && len(segment) >= 14 && bytes.Equal(segment[:6], []byte("Exif\x00\x00")) {
			if thumb, ok := exifJPEGThumbnail(segment[6:]); ok {
				return thumb, nil
			}
		}
		i += length
	}
	return nil, errors.New("未找到内嵌 JPEG 缩略图")
}

func exifJPEGThumbnail(tiff []byte) ([]byte, bool) {
	if len(tiff) < 8 || (string(tiff[:2]) != "II" && string(tiff[:2]) != "MM") {
		return nil, false
	}
	little := string(tiff[:2]) == "II"
	u16 := func(off int) uint16 {
		if off < 0 || off+2 > len(tiff) {
			return 0
		}
		if little {
			return uint16(tiff[off]) | uint16(tiff[off+1])<<8
		}
		return uint16(tiff[off])<<8 | uint16(tiff[off+1])
	}
	u32 := func(off int) uint32 {
		if off < 0 || off+4 > len(tiff) {
			return 0
		}
		if little {
			return uint32(tiff[off]) | uint32(tiff[off+1])<<8 | uint32(tiff[off+2])<<16 | uint32(tiff[off+3])<<24
		}
		return uint32(tiff[off])<<24 | uint32(tiff[off+1])<<16 | uint32(tiff[off+2])<<8 | uint32(tiff[off+3])
	}
	if u16(2) != 42 {
		return nil, false
	}
	ifd := int(u32(4))
	if ifd < 0 || ifd+2 > len(tiff) {
		return nil, false
	}
	count := int(u16(ifd))
	nextAt := ifd + 2 + count*12
	if nextAt+4 > len(tiff) {
		return nil, false
	}
	ifd = int(u32(nextAt)) // IFD1 contains the JPEG thumbnail pointers.
	if ifd <= 0 || ifd+2 > len(tiff) {
		return nil, false
	}
	count = int(u16(ifd))
	var offset, length uint32
	for n := 0; n < count; n++ {
		entry := ifd + 2 + n*12
		if entry+12 > len(tiff) {
			break
		}
		tag, typ := u16(entry), u16(entry+2)
		if typ != 4 || u32(entry+4) != 1 {
			continue
		}
		switch tag {
		case 0x0201:
			offset = u32(entry + 8)
		case 0x0202:
			length = u32(entry + 8)
		}
	}
	if offset == 0 || length < 16 || uint64(offset)+uint64(length) > uint64(len(tiff)) {
		return nil, false
	}
	thumb := tiff[offset : offset+length]
	if _, _, err := image.DecodeConfig(bytes.NewReader(thumb)); err != nil {
		return nil, false
	}
	return bytes.Clone(thumb), true
}

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
