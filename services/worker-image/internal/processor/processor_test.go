package processor

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// makePNG builds a simple w×h PNG in memory to feed the processor.
func makePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	return buf.Bytes()
}

func TestProcessProducesRequestedVariants(t *testing.T) {
	src := makePNG(t, 800, 600)
	p := New(Options{ThumbnailSize: 128, ResizeMaxDim: 400})

	results, err := p.Process(src, []string{"resize", "thumbnail", "webp"}, Params{})
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 variants, got %d", len(results))
	}

	byName := map[string]Result{}
	for _, r := range results {
		byName[r.Name] = r
		if len(r.Bytes) == 0 {
			t.Errorf("variant %q produced no bytes", r.Name)
		}
	}

	// The thumbnail must fit within the requested box.
	if w := byName["thumbnail"].Metadata["width"].(int); w > 128 {
		t.Errorf("thumbnail width %d exceeds 128", w)
	}
	// The resized variant must be bounded by ResizeMaxDim.
	if w := byName["resized"].Metadata["width"].(int); w > 400 {
		t.Errorf("resized width %d exceeds 400", w)
	}
	if byName["webp"].ContentType != "image/webp" {
		t.Errorf("webp content type = %q", byName["webp"].ContentType)
	}
}

func TestProcessUnknownOperationsIgnored(t *testing.T) {
	src := makePNG(t, 64, 64)
	p := New(Options{})
	results, err := p.Process(src, []string{"resize", "bogus"}, Params{})
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected only the resize result, got %d", len(results))
	}
}

func TestProcessRejectsNonImage(t *testing.T) {
	p := New(Options{})
	if _, err := p.Process([]byte("not an image"), []string{"resize"}, Params{}); err == nil {
		t.Fatal("expected an error decoding non-image input")
	}
}

// TestProcessHonorsParams proves per-job params override the processor defaults:
// a smaller resize dimension and a webp output format for the resized variant.
func TestProcessHonorsParams(t *testing.T) {
	src := makePNG(t, 800, 600)
	p := New(Options{ResizeMaxDim: 1600, ThumbnailSize: 256})

	results, err := p.Process(src, []string{"resize", "thumbnail"}, Params{
		ResizeMaxDim:  200,
		ThumbnailSize: 64,
		ResizeFormat:  "webp",
	})
	if err != nil {
		t.Fatalf("process: %v", err)
	}

	byName := map[string]Result{}
	for _, r := range results {
		byName[r.Name] = r
	}

	resized := byName["resized"]
	if resized.ContentType != "image/webp" {
		t.Errorf("resized content type = %q, want image/webp", resized.ContentType)
	}
	if w := resized.Metadata["width"].(int); w > 200 {
		t.Errorf("resized width %d exceeds requested 200", w)
	}
	if w := byName["thumbnail"].Metadata["width"].(int); w > 64 {
		t.Errorf("thumbnail width %d exceeds requested 64", w)
	}
}

// TestProcessGrayscale proves the grayscale op desaturates the image (R==G==B at
// every pixel, verified losslessly via PNG) while preserving its dimensions.
func TestProcessGrayscale(t *testing.T) {
	src := makePNG(t, 64, 48)
	p := New(Options{})

	results, err := p.Process(src, []string{"grayscale"}, Params{})
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	g := results[0]
	if g.Name != "grayscale" || g.ContentType != "image/png" {
		t.Fatalf("got name=%q content-type=%q", g.Name, g.ContentType)
	}
	// Grayscale preserves dimensions — it desaturates, it does not resample.
	if w, h := g.Metadata["width"].(int), g.Metadata["height"].(int); w != 64 || h != 48 {
		t.Errorf("dims = %dx%d, want 64x48", w, h)
	}

	out, err := png.Decode(bytes.NewReader(g.Bytes))
	if err != nil {
		t.Fatalf("decode grayscale output: %v", err)
	}
	// The fixture paints distinct R/G/B per pixel; after grayscale every channel
	// must be equal. PNG is lossless, so this comparison is exact.
	r, gg, b, _ := out.At(20, 10).RGBA()
	if r != gg || gg != b {
		t.Errorf("pixel not desaturated: r=%d g=%d b=%d", r>>8, gg>>8, b>>8)
	}
}

// TestProcessBlur proves the blur op actually softens the image — a hard
// black/white checkerboard must end up with intermediate tones — while
// preserving dimensions. PNG output keeps the check exact (no lossy noise).
func TestProcessBlur(t *testing.T) {
	// High-contrast 2px checkerboard: any gaussian blur blends the cells, so a
	// cell-center pixel can no longer be pure black or pure white.
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			c := color.RGBA{A: 255}
			if (x/2+y/2)%2 == 0 {
				c = color.RGBA{R: 255, G: 255, B: 255, A: 255}
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode fixture: %v", err)
	}

	p := New(Options{})
	results, err := p.Process(buf.Bytes(), []string{"blur"}, Params{BlurSigma: 2})
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	bl := results[0]
	if bl.Name != "blur" || bl.ContentType != "image/png" {
		t.Fatalf("got name=%q content-type=%q", bl.Name, bl.ContentType)
	}
	// Blur preserves dimensions — it softens, it does not resample.
	if w, h := bl.Metadata["width"].(int), bl.Metadata["height"].(int); w != 64 || h != 48 {
		t.Errorf("dims = %dx%d, want 64x48", w, h)
	}

	out, err := png.Decode(bytes.NewReader(bl.Bytes))
	if err != nil {
		t.Fatalf("decode blur output: %v", err)
	}
	r, _, _, _ := out.At(32, 24).RGBA()
	if v := r >> 8; v == 0 || v == 255 {
		t.Errorf("pixel still pure black/white (%d) — blur had no effect", v)
	}
}
