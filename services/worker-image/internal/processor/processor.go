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
	Name        string         // logical name: "resized", "thumbnail", "webp", "grayscale"
	ContentType string         //
	Bytes       []byte         //
	Metadata    map[string]any // width/height etc.
}

// Options tunes the transformations.
type Options struct {
	ThumbnailSize int // longest edge of the square thumbnail
	ResizeMaxDim  int // longest edge of the "resized" variant
}

// Params carries optional per-job overrides resolved from the job's "params"
// object. Zero-valued fields mean "use the processor's configured default".
type Params struct {
	ResizeMaxDim  int    // longest edge of the resized variant
	ThumbnailSize int    // longest edge of the thumbnail
	ResizeFormat  string // jpeg | png | webp; empty = jpeg
	Quality       int    // 1..100 for lossy formats; 0 = default
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
// artifacts. The per-job params override the processor defaults; pass the zero
// Params to use defaults. Unknown operations are ignored.
func (p *Processor) Process(src []byte, operations []string, params Params) ([]Result, error) {
	img, format, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("decode image (format=%s): %w", format, err)
	}

	maxDim := p.opts.ResizeMaxDim
	if params.ResizeMaxDim > 0 {
		maxDim = params.ResizeMaxDim
	}
	thumbSize := p.opts.ThumbnailSize
	if params.ThumbnailSize > 0 {
		thumbSize = params.ThumbnailSize
	}

	var results []Result
	for _, op := range operations {
		switch op {
		case "resize":
			r, err := p.resize(img, maxDim, params.ResizeFormat, params.Quality)
			if err != nil {
				return nil, err
			}
			results = append(results, r)
		case "thumbnail":
			r, err := p.thumbnail(img, thumbSize)
			if err != nil {
				return nil, err
			}
			results = append(results, r)
		case "webp":
			r, err := p.webp(img, maxDim)
			if err != nil {
				return nil, err
			}
			results = append(results, r)
		case "grayscale":
			r, err := p.grayscale(img)
			if err != nil {
				return nil, err
			}
			results = append(results, r)
		}
	}
	return results, nil
}

// resize produces the "resized" variant in the requested format (default JPEG).
func (p *Processor) resize(img image.Image, maxDim int, format string, quality int) (Result, error) {
	dst := imaging.Fit(img, maxDim, maxDim, imaging.Lanczos)
	buf := new(bytes.Buffer)

	switch format {
	case "png":
		if err := imaging.Encode(buf, dst, imaging.PNG); err != nil {
			return Result{}, fmt.Errorf("encode resized (png): %w", err)
		}
		return Result{Name: "resized", ContentType: "image/png", Bytes: buf.Bytes(), Metadata: dims(dst)}, nil
	case "webp":
		if err := nativewebp.Encode(buf, dst, nil); err != nil {
			return Result{}, fmt.Errorf("encode resized (webp): %w", err)
		}
		return Result{Name: "resized", ContentType: "image/webp", Bytes: buf.Bytes(), Metadata: dims(dst)}, nil
	default: // jpeg
		q := quality
		if q <= 0 {
			q = 85
		}
		if err := imaging.Encode(buf, dst, imaging.JPEG, imaging.JPEGQuality(q)); err != nil {
			return Result{}, fmt.Errorf("encode resized (jpeg): %w", err)
		}
		return Result{Name: "resized", ContentType: "image/jpeg", Bytes: buf.Bytes(), Metadata: dims(dst)}, nil
	}
}

func (p *Processor) thumbnail(img image.Image, size int) (Result, error) {
	dst := imaging.Thumbnail(img, size, size, imaging.Lanczos)
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

func (p *Processor) webp(img image.Image, maxDim int) (Result, error) {
	// Bound the webp variant to the resize dimension to keep payloads sane.
	dst := imaging.Fit(img, maxDim, maxDim, imaging.Lanczos)
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

// grayscale desaturates the image to luminance, preserving its dimensions. It is
// encoded as lossless PNG so the output is a faithful tonal copy with no resampling.
func (p *Processor) grayscale(img image.Image) (Result, error) {
	dst := imaging.Grayscale(img)
	buf := new(bytes.Buffer)
	if err := imaging.Encode(buf, dst, imaging.PNG); err != nil {
		return Result{}, fmt.Errorf("encode grayscale: %w", err)
	}
	return Result{
		Name:        "grayscale",
		ContentType: "image/png",
		Bytes:       buf.Bytes(),
		Metadata:    dims(dst),
	}, nil
}

func dims(img image.Image) map[string]any {
	b := img.Bounds()
	return map[string]any{"width": b.Dx(), "height": b.Dy()}
}
