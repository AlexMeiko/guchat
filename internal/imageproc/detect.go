package imageproc

import (
	"bytes"
	"errors"
)

var ErrUnsupportedFormat = errors.New("unsupported image format")

const (
	MIMEJPEG = "image/jpeg"
	MIMEPNG  = "image/png"
	MIMEWebP = "image/webp"
)

var (
	jpegSOI      = []byte{0xff, 0xd8, 0xff}
	jpegXLMarker = byte(0xf7)
	pngSignature = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	riffHeader   = []byte("RIFF")
	webpHeader   = []byte("WEBP")
)

func Detect(data []byte) (string, error) {
	switch {
	case bytes.HasPrefix(data, jpegSOI):
		if len(data) >= 4 && data[3] == jpegXLMarker {
			return "", ErrUnsupportedFormat
		}
		return MIMEJPEG, nil
	case bytes.HasPrefix(data, pngSignature):
		return MIMEPNG, nil
	case len(data) >= 12 && bytes.HasPrefix(data, riffHeader) && bytes.Equal(data[8:12], webpHeader):
		return MIMEWebP, nil
	default:
		return "", ErrUnsupportedFormat
	}
}
