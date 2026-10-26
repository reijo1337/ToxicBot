package linkpreview

import (
	"bytes"
	"errors"
	"image"
	_ "image/jpeg" // Registers the supported JPEG decoder.
	_ "image/png"  // Registers the supported PNG decoder.
	"io"
	"mime"
	"net/http"
)

const (
	maxImageBytes  = 5 * 1024 * 1024
	maxImagePixels = 20_000_000
)

func readImage(body io.Reader, contentType string) ([]byte, error) {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || (mediaType != "image/jpeg" && mediaType != "image/png") {
		return nil, errors.New("content_type")
	}
	data, err := io.ReadAll(io.LimitReader(body, maxImageBytes+1))
	if err != nil {
		return nil, errors.New("image_read")
	}
	if len(data) > maxImageBytes {
		return nil, errors.New("image_limit")
	}
	if http.DetectContentType(data) != mediaType {
		return nil, errors.New("image_type")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") {
		return nil, errors.New("image_header")
	}
	if config.Width <= 0 || config.Height <= 0 ||
		int64(config.Width) > maxImagePixels/int64(config.Height) {
		return nil, errors.New("image_geometry")
	}
	return data, nil
}
