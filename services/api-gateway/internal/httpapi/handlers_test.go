package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/viniciusoliveira/mediaforge/api-gateway/internal/media"
	"github.com/viniciusoliveira/mediaforge/api-gateway/internal/store"
)

// formReq builds a request whose form values come from the query string.
// r.FormValue parses the query, which is all parseParams reads.
func formReq(query string) *http.Request {
	return httptest.NewRequest(http.MethodPost, "/v1/media?"+query, nil)
}

// ---- GET /v1/media (gallery listing) ----

// fakeJobs records the paging it was asked for and serves canned pages.
type fakeJobs struct {
	items              []store.JobListItem
	total              int
	gotLimit, gotOffset int
}

func (f *fakeJobs) CreateJob(context.Context, *media.Job) error { return nil }
func (f *fakeJobs) GetJob(context.Context, string) (*media.Job, []media.Artifact, error) {
	return nil, nil, store.ErrNotFound
}
func (f *fakeJobs) ListJobs(_ context.Context, limit, offset int) ([]store.JobListItem, int, error) {
	f.gotLimit, f.gotOffset = limit, offset
	return f.items, f.total, nil
}

// fakeObjects presigns by prefixing the key, so URLs are assertable.
type fakeObjects struct{}

func (fakeObjects) Put(_ context.Context, _, _ string, _ io.Reader, size int64) (int64, error) {
	return size, nil
}
func (fakeObjects) PresignedGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://signed.local/" + key, nil
}

type listResponse struct {
	Jobs []struct {
		Job      media.Job `json:"job"`
		ThumbURL string    `json:"thumb_url"`
	} `json:"jobs"`
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

func listServer(jobs *fakeJobs) *Server {
	return &Server{
		Jobs:    jobs,
		Objects: fakeObjects{},
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func doList(t *testing.T, s *Server, target string) (*httptest.ResponseRecorder, listResponse) {
	t.Helper()
	rec := httptest.NewRecorder()
	s.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	var body listResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal list response: %v", err)
		}
	}
	return rec, body
}

func TestHandleList_DefaultsAndShape(t *testing.T) {
	jobs := &fakeJobs{
		items: []store.JobListItem{
			{Job: media.Job{ID: "a1", Kind: media.KindImage, Status: media.StatusCompleted}, ThumbKey: "outputs/a1/thumb.png"},
			{Job: media.Job{ID: "b2", Kind: media.KindOCR, Status: media.StatusFailed}},
		},
		total: 2,
	}
	rec, body := doList(t, listServer(jobs), "/v1/media")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if jobs.gotLimit != 24 || jobs.gotOffset != 0 {
		t.Errorf("paging = (%d,%d), want defaults (24,0)", jobs.gotLimit, jobs.gotOffset)
	}
	if body.Total != 2 || len(body.Jobs) != 2 {
		t.Fatalf("total=%d len=%d, want 2/2", body.Total, len(body.Jobs))
	}
	if body.Jobs[0].Job.ID != "a1" {
		t.Errorf("jobs[0].job.job_id = %q, want a1 (nested job object)", body.Jobs[0].Job.ID)
	}
	if body.Jobs[0].ThumbURL != "https://signed.local/outputs/a1/thumb.png" {
		t.Errorf("thumb_url = %q, want presigned thumbnail", body.Jobs[0].ThumbURL)
	}
	if body.Jobs[1].ThumbURL != "" {
		t.Errorf("thumb_url = %q, want empty when job has no thumbnail", body.Jobs[1].ThumbURL)
	}
}

func TestHandleList_PagingPassedThrough(t *testing.T) {
	jobs := &fakeJobs{}
	_, body := doList(t, listServer(jobs), "/v1/media?limit=5&offset=10")
	if jobs.gotLimit != 5 || jobs.gotOffset != 10 {
		t.Errorf("paging = (%d,%d), want (5,10)", jobs.gotLimit, jobs.gotOffset)
	}
	if body.Limit != 5 || body.Offset != 10 {
		t.Errorf("echoed paging = (%d,%d), want (5,10)", body.Limit, body.Offset)
	}
}

func TestHandleList_BadPagingFallsBack(t *testing.T) {
	// Oversized limit clamps to the max (so a client paging by it never skips
	// rows); junk and negatives fall back to defaults instead of 400.
	jobs := &fakeJobs{}
	doList(t, listServer(jobs), "/v1/media?limit=9999&offset=-3")
	if jobs.gotLimit != 100 || jobs.gotOffset != 0 {
		t.Errorf("paging = (%d,%d), want clamped (100,0)", jobs.gotLimit, jobs.gotOffset)
	}
	doList(t, listServer(jobs), "/v1/media?limit=abc&offset=xyz")
	if jobs.gotLimit != 24 || jobs.gotOffset != 0 {
		t.Errorf("paging = (%d,%d), want defaults (24,0)", jobs.gotLimit, jobs.gotOffset)
	}
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
