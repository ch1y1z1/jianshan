package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"
)

// parseJPEGMetadata reads common EXIF fields without executing external code.
// Missing or malformed fields remain unknown.
func parseJPEGMetadata(data []byte, m metadata) metadata {
	if len(data) < 4 || data[0] != 0xff || data[1] != 0xd8 {
		return m
	}
	width, height := m.Width, m.Height
	for i := 2; i+4 <= len(data); {
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
		if marker == 0xda || marker == 0xd9 { // image data / end
			break
		}
		if marker >= 0xd0 && marker <= 0xd7 || marker == 0x01 {
			continue
		}
		if i+2 > len(data) {
			break
		}
		length := int(binary.BigEndian.Uint16(data[i : i+2]))
		if length < 2 || i+length > len(data) {
			break
		}
		segment := data[i+2 : i+length]
		if marker == 0xe1 && len(segment) >= 6 && string(segment[:6]) == "Exif\x00\x00" {
			m = parseTIFF(segment[6:], m)
			break
		}
		i += length
	}
	// A resized JPEG may retain the original EXIF pixel dimensions. Its
	// decoded header remains authoritative for the actual file dimensions.
	if width > 0 && height > 0 {
		m.Width, m.Height = width, height
	}
	return m
}

type tiffReader struct {
	b      []byte
	little bool
}

func parseTIFF(data []byte, m metadata) metadata {
	if len(data) < 8 || (string(data[:2]) != "II" && string(data[:2]) != "MM") {
		return m
	}
	r := tiffReader{b: data, little: string(data[:2]) == "II"}
	if r.u16(2) != 42 {
		return m
	}
	ifd := int(r.u32(4))
	return r.readIFD(ifd, m, 0)
}

func (r tiffReader) u16(off int) uint16 {
	if off < 0 || off+2 > len(r.b) {
		return 0
	}
	if r.little {
		return binary.LittleEndian.Uint16(r.b[off : off+2])
	}
	return binary.BigEndian.Uint16(r.b[off : off+2])
}

func (r tiffReader) u32(off int) uint32 {
	if off < 0 || off+4 > len(r.b) {
		return 0
	}
	if r.little {
		return binary.LittleEndian.Uint32(r.b[off : off+4])
	}
	return binary.BigEndian.Uint32(r.b[off : off+4])
}

func (r tiffReader) readIFD(off int, m metadata, depth int) metadata {
	if depth > 4 || off < 0 || off+2 > len(r.b) {
		return m
	}
	count := int(r.u16(off))
	base := off + 2
	for i := 0; i < count; i++ {
		entry := base + i*12
		if entry+12 > len(r.b) {
			break
		}
		tag := r.u16(entry)
		typ := r.u16(entry + 2)
		n := r.u32(entry + 4)
		value := entry + 8
		unit := tiffTypeSize(typ)
		if unit == 0 {
			continue
		}
		if n == 0 || uint64(n)*uint64(unit) > uint64(len(r.b)) {
			continue
		}
		total := int(n) * unit
		if total > 4 {
			value = int(r.u32(value))
		}
		if value < 0 || value+max(total, 1) > len(r.b) {
			continue
		}
		switch tag {
		case 0x010f, 0x0110: // Make, Model
			v := strings.TrimSpace(r.ascii(value, total))
			if v != "" {
				if tag == 0x010f && m.Camera == "" {
					m.Camera = v
				} else if tag == 0x0110 {
					if m.Camera == "" {
						m.Camera = v
					} else if !strings.Contains(m.Camera, v) {
						m.Camera += " " + v
					}
				}
			}
		case 0x0112:
			if typ == 3 && n == 1 && m.Orientation == 0 {
				m.Orientation = int(r.u16(value))
			}
		case 0x0100, 0xa002:
			m.Width = max(m.Width, r.integer(value, typ))
		case 0x0101, 0xa003:
			m.Height = max(m.Height, r.integer(value, typ))
		case 0x0132, 0x9003:
			if t, ok := parseEXIFTime(strings.TrimSpace(r.ascii(value, total))); ok && (m.Captured.IsZero() || tag == 0x9003) {
				m.Captured = t
			}
		case 0x8827:
			if n > 0 {
				iso := r.integer(value, typ)
				if iso > 0 {
					m.ISO = fmt.Sprintf("ISO %d", iso)
				}
			}
		case 0x829a:
			if num, den, ok := r.rational(value); typ == 5 && n == 1 && ok && num > 0 {
				m.Shutter = formatExposure(num, den)
			}
		case 0x829d:
			if num, den, ok := r.rational(value); typ == 5 && n == 1 && ok && num > 0 {
				m.Aperture = fmt.Sprintf("f/%s", formatNumber(num, den))
			}
		case 0x920a:
			if num, den, ok := r.rational(value); typ == 5 && n == 1 && ok && num > 0 {
				m.FocalLength = formatNumber(num, den) + " mm"
			}
		case 0xa434:
			m.Lens = strings.TrimSpace(r.ascii(value, total))
		case 0x8769, 0x014a: // EXIF and RAW image SubIFDs
			if typ == 4 {
				for k := 0; k < min(int(n), 16); k++ {
					m = r.readIFD(int(r.u32(value+k*4)), m, depth+1)
				}
			}
		}
	}
	if next := int(r.u32(base + count*12)); next > 0 {
		m = r.readIFD(next, m, depth+1)
	}
	return m
}

func tiffTypeSize(typ uint16) int {
	switch typ {
	case 1, 2, 6, 7:
		return 1
	case 3, 8:
		return 2
	case 4, 9, 11:
		return 4
	case 5, 10, 12:
		return 8
	default:
		return 0
	}
}

func (r tiffReader) ascii(off, n int) string {
	if off < 0 || off >= len(r.b) {
		return ""
	}
	end := min(len(r.b), off+n)
	return strings.TrimRight(string(r.b[off:end]), "\x00")
}

func (r tiffReader) integer(off int, typ uint16) int {
	switch typ {
	case 3:
		return int(r.u16(off))
	case 4:
		return int(r.u32(off))
	default:
		return 0
	}
}

func (r tiffReader) rational(off int) (float64, float64, bool) {
	if off < 0 || off+8 > len(r.b) {
		return 0, 0, false
	}
	var num, den uint32
	if r.little {
		num, den = binary.LittleEndian.Uint32(r.b[off:off+4]), binary.LittleEndian.Uint32(r.b[off+4:off+8])
	} else {
		num, den = binary.BigEndian.Uint32(r.b[off:off+4]), binary.BigEndian.Uint32(r.b[off+4:off+8])
	}
	return float64(num), float64(den), den != 0
}

func formatNumber(num, den float64) string {
	v := num / den
	if math.Abs(v-math.Round(v)) < 0.01 {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.1f", v)
}

func formatExposure(num, den float64) string {
	v := num / den
	if v >= 1 {
		return formatNumber(num, den) + " s"
	}
	return fmt.Sprintf("1/%d s", int(math.Round(1/v)))
}

func parseEXIFTime(s string) (time.Time, bool) {
	t, err := time.Parse("2006:01:02 15:04:05", s)
	return t, err == nil
}
