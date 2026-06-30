package image

import (
	"bytes"
	"image"
	"image/jpeg"
	"io"
	"math"

	"golang.org/x/image/draw"
)

const (
	maxDimension = 1024
	jpegQuality  = 85
)

// Normalizer handles image normalization for VLLM processing.
type Normalizer struct {
	maxDimension int
	jpegQuality  int
}

// NewNormalizer creates a new image normalizer with default settings.
func NewNormalizer() *Normalizer {
	return &Normalizer{
		maxDimension: maxDimension,
		jpegQuality:  jpegQuality,
	}
}

// NewNormalizerWithOptions creates a normalizer with custom options.
func NewNormalizerWithOptions(maxDim, quality int) *Normalizer {
	return &Normalizer{
		maxDimension: maxDim,
		jpegQuality:  quality,
	}
}

// NormalizeImage normalizes a single image: resizes if needed and optimizes quality.
func (n *Normalizer) NormalizeImage(input io.Reader) ([]byte, error) {
	// Decode image
	img, _, err := image.Decode(input)
	if err != nil {
		return nil, err
	}

	// Resize if needed
	resized := n.resize(img)

	// Encode as JPEG with specified quality
	var buf bytes.Buffer
	err = jpeg.Encode(&buf, resized, &jpeg.Options{Quality: n.jpegQuality})
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// resize resizes an image if it exceeds maxDimension while maintaining aspect ratio.
func (n *Normalizer) resize(img image.Image) image.Image {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Check if resizing is needed
	if width <= n.maxDimension && height <= n.maxDimension {
		return img
	}

	// Calculate new dimensions maintaining aspect ratio
	var newWidth, newHeight int
	if width > height {
		newWidth = n.maxDimension
		newHeight = int(math.Round(float64(height) * float64(n.maxDimension) / float64(width)))
	} else {
		newHeight = n.maxDimension
		newWidth = int(math.Round(float64(width) * float64(n.maxDimension) / float64(height)))
	}

	// Create new image and resize
	resized := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	draw.CatmullRom.Scale(resized, resized.Bounds(), img, bounds, draw.Over, nil)

	return resized
}

// GetImageDimensions returns the dimensions of an image.
func (n *Normalizer) GetImageDimensions(input io.Reader) (width, height int, err error) {
	img, _, err := image.Decode(input)
	if err != nil {
		return 0, 0, err
	}
	bounds := img.Bounds()
	return bounds.Dx(), bounds.Dy(), nil
}
