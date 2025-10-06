//go:build !darwin

package screenshots

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"time"
)

type syntheticProvider struct{}

func defaultCaptureProvider() (CaptureProvider, error) {
	rand.Seed(time.Now().UnixNano())
	return syntheticProvider{}, nil
}

func (syntheticProvider) Grab(ctx context.Context) (FrameCapture, error) {
	const width, height = 640, 400
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	hue := uint8(rand.Intn(200) + 40)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, color.RGBA{R: hue, G: uint8(x % 255), B: uint8(y % 255), A: 255})
		}
	}
	buf := &bytes.Buffer{}
	if err := png.Encode(buf, img); err != nil {
		return FrameCapture{}, err
	}
	return FrameCapture{
		PNG: buf.Bytes(),
		Metadata: Metadata{
			CapturedAt:  time.Now().UTC(),
			Backend:     "synthetic",
			Width:       width,
			Height:      height,
			PixelFormat: "RGBA",
			Scale:       1,
		},
	}, nil
}
