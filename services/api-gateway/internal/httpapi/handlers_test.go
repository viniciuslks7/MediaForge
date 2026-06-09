package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// formReq builds a request whose form values come from the query string.
// r.FormValue parses the query, which is all parseParams reads.
func formReq(query string) *http.Request {
	return httptest.NewRequest(http.MethodPost, "/v1/media?"+query, nil)
}

func TestParseParams_NoneReturnsNil(t *testing.T) {
	if p := parseParams(formReq("")); p != nil {
		t.Fatalf("expected nil for no params, got %+v", p)
	}
}

func TestParseParams_AllValid(t *testing.T) {
	p := parseParams(formReq("resize_max_dim=800&thumbnail_size=128&resize_format=webp&quality=90&blur_sigma=2.5"))
	if p == nil {
		t.Fatal("expected params, got nil")
	}
	if p.ResizeMaxDim != 800 {
		t.Errorf("resize_max_dim = %d, want 800", p.ResizeMaxDim)
	}
	if p.ThumbnailSize != 128 {
		t.Errorf("thumbnail_size = %d, want 128", p.ThumbnailSize)
	}
	if p.ResizeFormat != "webp" {
		t.Errorf("resize_format = %q, want webp", p.ResizeFormat)
	}
	if p.Quality != 90 {
		t.Errorf("quality = %d, want 90", p.Quality)
	}
	if p.BlurSigma != 2.5 {
		t.Errorf("blur_sigma = %v, want 2.5", p.BlurSigma)
	}
}

func TestParseParams_OutOfRangeDropped(t *testing.T) {
	// All values are out of the schema's bounds, so none should be set and the
	// whole result collapses to nil (worker falls back to its defaults).
	p := parseParams(formReq("resize_max_dim=999999&thumbnail_size=5&quality=0&blur_sigma=100"))
	if p != nil {
		t.Fatalf("expected nil when all params out of range, got %+v", p)
	}
}

func TestParseParams_InvalidFormatIgnored(t *testing.T) {
	// An unknown format is dropped; with no other fields the result is nil.
	if p := parseParams(formReq("resize_format=gif")); p != nil {
		t.Fatalf("expected nil for invalid format, got %+v", p)
	}
}

func TestParseParams_NonNumericIgnored(t *testing.T) {
	if p := parseParams(formReq("resize_max_dim=abc")); p != nil {
		t.Fatalf("expected nil for non-numeric dim, got %+v", p)
	}
}

func TestParseParams_PartialKeepsValidOnly(t *testing.T) {
	// One valid field alongside an out-of-range one: only the valid field is set.
	p := parseParams(formReq("thumbnail_size=256&quality=500"))
	if p == nil {
		t.Fatal("expected params, got nil")
	}
	if p.ThumbnailSize != 256 {
		t.Errorf("thumbnail_size = %d, want 256", p.ThumbnailSize)
	}
	if p.Quality != 0 {
		t.Errorf("quality = %d, want 0 (out-of-range dropped)", p.Quality)
	}
}
