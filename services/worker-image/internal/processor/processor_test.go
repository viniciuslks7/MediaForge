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
