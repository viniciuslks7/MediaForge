// Package processor performs the actual image transformations. It is pure (no
// I/O): given the source bytes and a set of operations it returns the produced
// artifacts in memory, which keeps it trivially unit-testable.
package processor

import (
	"bytes"
	"fmt"
	"image"

	"github.com/HugoSmits86/nativewebp"
	"github.com/disintegration/imaging"
)

// Result is one produced image variant.
type Result struct {
	Name        string         // logical name: "resized", "thumbnail", "webp"
	ContentType string         //
	Bytes       []byte         //
	Metadata    map[string]any // width/height etc.
}

// Options tunes the transformations.
type Options struct {
	ThumbnailSize int // longest edge of the square thumbnail
	ResizeMaxDim  int // longest edge of the "resized" variant
}

// Processor applies a set of operations to a decoded image.
type Processor struct{ opts Options }

func New(opts Options) *Processor {
	if opts.ThumbnailSize <= 0 {
		opts.ThumbnailSize = 256
	}
	if opts.ResizeMaxDim <= 0 {
		opts.ResizeMaxDim = 1600
	}
	return &Processor{opts: opts}
}

// Process decodes src and runs each requested operation, returning the produced
// artifacts. Unknown operations are ignored.
func (p *Processor) Process(src []byte, operations []string) ([]Result, error) {
	img, format, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("decode image (format=%s): %w", format, err)
	}

	var results []Result
	for _, op := range operations {
		switch op {
		case "resize":
			r, err := p.resize(img)
			if err != nil {
				return nil, err
			}
			results = append(results, r)
		case "thumbnail":
			r, err := p.thumbnail(img)
			if err != nil {
				return nil, err
			}
			results = append(results, r)
		case "webp":
			r, err := p.webp(img)
			if err != nil {
				return nil, err
			}
			results = append(results, r)
		}
	}
	return results, nil
}

func (p *Processor) resize(img image.Image) (Result, error) {
	dst := imaging.Fit(img, p.opts.ResizeMaxDim, p.opts.ResizeMaxDim, imaging.Lanczos)
	buf := new(bytes.Buffer)
	if err := imaging.Encode(buf, dst, imaging.JPEG, imaging.JPEGQuality(85)); err != nil {
		return Result{}, fmt.Errorf("encode resized: %w", err)
	}
	return Result{
		Name:        "resized",
		ContentType: "image/jpeg",
		Bytes:       buf.Bytes(),
		Metadata:    dims(dst),
	}, nil
}

func (p *Processor) thumbnail(img image.Image) (Result, error) {
	dst := imaging.Thumbnail(img, p.opts.ThumbnailSize, p.opts.ThumbnailSize, imaging.Lanczos)
	buf := new(bytes.Buffer)
	if err := imaging.Encode(buf, dst, imaging.PNG); err != nil {
		return Result{}, fmt.Errorf("encode thumbnail: %w", err)
	}
	return Result{
		Name:        "thumbnail",
		ContentType: "image/png",
		Bytes:       buf.Bytes(),
		Metadata:    dims(dst),
	}, nil
}

func (p *Processor) webp(img image.Image) (Result, error) {
	// Bound the webp variant to the resize dimension to keep payloads sane.
	dst := imaging.Fit(img, p.opts.ResizeMaxDim, p.opts.ResizeMaxDim, imaging.Lanczos)
	buf := new(bytes.Buffer)
	if err := nativewebp.Encode(buf, dst, nil); err != nil {
		return Result{}, fmt.Errorf("encode webp: %w", err)
	}
	return Result{
		Name:        "webp",
		ContentType: "image/webp",
		Bytes:       buf.Bytes(),
		Metadata:    dims(dst),
	}, nil
}

func dims(img image.Image) map[string]any {
	b := img.Bounds()
	return map[string]any{"width": b.Dx(), "height": b.Dy()}
}
