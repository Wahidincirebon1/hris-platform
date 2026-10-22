package httpapi

import (
	"image"
	"image/color"
	"testing"
)

func TestCropAndResizePhotoCreatesSquareImage(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 800, 400))
	for y := 0; y < source.Bounds().Dy(); y++ {
		for x := 0; x < source.Bounds().Dx(); x++ {
			source.SetRGBA(x, y, color.RGBA{R: 210, G: 110, B: 80, A: 255})
		}
	}
	source.SetRGBA(400, 200, color.RGBA{R: 20, G: 40, B: 60, A: 255})
	result := cropAndResizePhoto(source, profilePhotoSize)
	if got := result.Bounds().Dx(); got != profilePhotoSize {
		t.Fatalf("width = %d, want %d", got, profilePhotoSize)
	}
	if got := result.Bounds().Dy(); got != profilePhotoSize {
		t.Fatalf("height = %d, want %d", got, profilePhotoSize)
	}
	if got := result.RGBAAt(profilePhotoSize/2, profilePhotoSize/2); got != (color.RGBA{R: 20, G: 40, B: 60, A: 255}) {
		t.Fatalf("center pixel = %#v, want centered source crop color", got)
	}
}

func TestCropAndResizePhotoFlattensTransparentPixels(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 2, 2))
	result := cropAndResizePhoto(source, 1)
	if got := result.RGBAAt(0, 0); got != (color.RGBA{R: 255, G: 255, B: 255, A: 255}) {
		t.Fatalf("transparent pixel = %#v, want white background", got)
	}
}
