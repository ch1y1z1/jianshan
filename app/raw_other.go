//go:build !darwin

package main

import (
	"context"
	"errors"
)

func platformRawPreview(ctx context.Context, path string) ([]byte, error) {
	return nil, errors.New("此 RAW 文件没有可用的内嵌 JPEG 预览")
}
