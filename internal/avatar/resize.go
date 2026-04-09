package avatar

import (
	"bytes"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"

	"golang.org/x/image/draw"
)

const targetSize = 256

// ProcessUpload decodes an uploaded image, center-crops it to a square,
// resizes to 256x256, and returns PNG bytes.
func ProcessUpload(data []byte, contentType string) ([]byte, string, error) {
	var img image.Image
	var err error

	switch contentType {
	case "image/png":
		img, err = png.Decode(bytes.NewReader(data))
	case "image/jpeg":
		img, err = jpeg.Decode(bytes.NewReader(data))
	case "image/gif":
		img, err = gif.Decode(bytes.NewReader(data))
	default:
		return nil, "", fmt.Errorf("unsupported image type: %s", contentType)
	}
	if err != nil {
		return nil, "", fmt.Errorf("decode image: %w", err)
	}

	cropped := centerCrop(img)

	dst := image.NewRGBA(image.Rect(0, 0, targetSize, targetSize))
	draw.CatmullRom.Scale(dst, dst.Bounds(), cropped, cropped.Bounds(), draw.Over, nil)

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, "", fmt.Errorf("encode png: %w", err)
	}

	return buf.Bytes(), "image/png", nil
}

// centerCrop returns a square sub-image taken from the center of img.
func centerCrop(img image.Image) image.Image {
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	if w == h {
		return img
	}

	var cropRect image.Rectangle
	if w > h {
		offset := (w - h) / 2
		cropRect = image.Rect(bounds.Min.X+offset, bounds.Min.Y, bounds.Min.X+offset+h, bounds.Max.Y)
	} else {
		offset := (h - w) / 2
		cropRect = image.Rect(bounds.Min.X, bounds.Min.Y+offset, bounds.Max.X, bounds.Min.Y+offset+w)
	}

	type subImager interface {
		SubImage(r image.Rectangle) image.Image
	}
	if si, ok := img.(subImager); ok {
		return si.SubImage(cropRect)
	}

	// Fallback: copy pixels manually.
	size := cropRect.Dx()
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Copy(dst, image.Point{}, img, cropRect, draw.Src, nil)
	return dst
}
