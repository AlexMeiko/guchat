package imageproc

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/jpeg"
	"image/png"

	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

const jpegQuality = 90

type Options struct {
	MaxEdge int
}

type Result struct {
	StoreKey       string
	OriginalMIME   string
	OriginalWidth  int
	OriginalHeight int
	OriginalSize   int64
	Bytes          []byte
	MIME           string
	Width          int
	Height         int
}

func Process(original []byte, opts Options) (*Result, error) {
	sum := sha256.Sum256(original)
	storeKey := hex.EncodeToString(sum[:])

	mime, err := Detect(original)
	if err != nil {
		return nil, err
	}

	img, err := decodeImage(original, mime)
	if err != nil {
		return nil, err
	}

	ow, oh := img.Bounds().Dx(), img.Bounds().Dy()
	if ow < 1 || oh < 1 {
		return nil, ErrUnsupportedFormat
	}

	tw, th := fitSize(ow, oh, opts.MaxEdge)
	if tw != ow || th != oh {
		img = resizeImage(img, tw, th)
	}

	var (
		encoded []byte
		outMIME string
	)
	if mime == MIMEJPEG {
		encoded, err = encodeJPEG(img)
		outMIME = MIMEJPEG
	} else {
		encoded, err = encodePNG(img)
		outMIME = MIMEPNG
	}
	if err != nil {
		return nil, err
	}

	return &Result{
		StoreKey:       storeKey,
		OriginalMIME:   mime,
		OriginalWidth:  ow,
		OriginalHeight: oh,
		OriginalSize:   int64(len(original)),
		Bytes:          encoded,
		MIME:           outMIME,
		Width:          tw,
		Height:         th,
	}, nil
}

func decodeImage(data []byte, mime string) (image.Image, error) {
	r := bytes.NewReader(data)
	switch mime {
	case MIMEJPEG:
		return jpeg.Decode(r)
	case MIMEPNG:
		return png.Decode(r)
	case MIMEWebP:
		return webp.Decode(r)
	default:
		return nil, ErrUnsupportedFormat
	}
}

func fitSize(w, h, maxEdge int) (int, int) {
	long := max(w, h)
	if long <= maxEdge {
		return w, h
	}
	return max(1, w*maxEdge/long), max(1, h*maxEdge/long)
}

func resizeImage(src image.Image, w, h int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	return dst
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func encodeJPEG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
