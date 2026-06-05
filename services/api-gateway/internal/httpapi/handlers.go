// Package httpapi is the HTTP surface of the gateway: routing, upload handling,
// job submission and status lookup.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/viniciusoliveira/mediaforge/api-gateway/internal/broker"
	"github.com/viniciusoliveira/mediaforge/api-gateway/internal/media"
	"github.com/viniciusoliveira/mediaforge/api-gateway/internal/observability"
	"github.com/viniciusoliveira/mediaforge/api-gateway/internal/store"
)

// Dependencies the handlers need; satisfied by the concrete adapters in main.
type (
	JobStore interface {
		CreateJob(ctx context.Context, j *media.Job) error
		GetJob(ctx context.Context, id string) (*media.Job, []media.Artifact, error)
	}
	ObjectStore interface {
		Put(ctx context.Context, key, contentType string, r io.Reader, size int64) (int64, error)
		PresignedGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	}
	Publisher interface {
		PublishJSON(ctx context.Context, exchange, key string, v any) error
	}
	RateLimiter interface {
		Allow(ctx context.Context, key string) (bool, error)
	}
)

type Server struct {
	Jobs           JobStore
	Objects        ObjectStore
	Bus            Publisher
	Limiter        RateLimiter
	Log            *slog.Logger
	MaxUploadBytes int64
	AuthToken      string
}

// Router builds the chi router with all middleware and routes wired up.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(instrument)

	r.Get("/healthz", s.handleHealth)
	r.Get("/readyz", s.handleHealth)
	r.Handle("/metrics", observability.MetricsHandler())

	r.Route("/v1/media", func(r chi.Router) {
		r.With(bearerAuth(s.AuthToken)).Post("/", s.handleSubmit)
		r.Get("/{id}", s.handleStatus)
	})

	// otelhttp wraps the whole router: it extracts inbound W3C trace context and
	// opens a server span per request. The name formatter runs before chi has
	// matched a route, so we name spans by method only — keeping URLs with job
	// IDs out of the span name avoids high-cardinality trace data.
	return otelhttp.NewHandler(r, "http.server",
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return r.Method
		}),
	)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleSubmit accepts a multipart upload, stores the original, persists a job
// and publishes it to the appropriate worker queue.
func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if s.Limiter != nil {
		ok, err := s.Limiter.Allow(ctx, clientKey(r))
		if err != nil {
			s.Log.Warn("rate limiter error", "err", err)
		} else if !ok {
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
	}

	r.Body = http.MaxBytesReader(w, r.Body, s.MaxUploadBytes)
	if err := r.ParseMultipartForm(s.MaxUploadBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart upload or file too large")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing 'file' field")
		return
	}
	defer file.Close()

	kind := resolveKind(r.FormValue("kind"), header.Filename)
	if !kind.Valid() {
		writeError(w, http.StatusBadRequest, "unsupported media kind")
		return
	}

	ops := parseOperations(r.FormValue("operations"))
	params := parseParams(r)
	jobID := uuid.NewString()
	contentType := detectContentType(header.Header.Get("Content-Type"), header.Filename)
	sourceKey := fmt.Sprintf("uploads/%s/%s", jobID, sanitize(header.Filename))

	size, err := s.Objects.Put(ctx, sourceKey, contentType, file, header.Size)
	if err != nil {
		s.Log.Error("store upload", "err", err, "job_id", jobID)
		writeError(w, http.StatusBadGateway, "failed to store upload")
		return
	}

	job := &media.Job{
		ID:         jobID,
		Kind:       kind,
		Status:     media.StatusPending,
		SourceKey:  sourceKey,
		SourceMIME: contentType,
		SizeBytes:  size,
		Operations: ops,
		Params:     params,
	}
	if err := s.Jobs.CreateJob(ctx, job); err != nil {
		s.Log.Error("persist job", "err", err, "job_id", jobID)
		writeError(w, http.StatusInternalServerError, "failed to persist job")
		return
	}

	if err := s.Bus.PublishJSON(ctx, broker.ExchangeJobs, kind.RoutingKey(), job); err != nil {
		s.Log.Error("publish job", "err", err, "job_id", jobID)
		writeError(w, http.StatusBadGateway, "failed to enqueue job")
		return
	}

	observability.JobsPublished.WithLabelValues(string(kind)).Inc()
	observability.UploadBytes.Observe(float64(size))
	s.Log.Info("job accepted", "job_id", jobID, "kind", kind, "bytes", size)

	writeJSON(w, http.StatusAccepted, map[string]any{
		"job_id": jobID,
		"kind":   kind,
		"status": media.StatusPending,
	})
}

// handleStatus returns the job, its artifacts and presigned download URLs.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	job, artifacts, err := s.Jobs.GetJob(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	if err != nil {
		s.Log.Error("load job", "err", err, "job_id", id)
		writeError(w, http.StatusInternalServerError, "failed to load job")
		return
	}

	type outArtifact struct {
		media.Artifact
		URL string `json:"url"`
	}
	out := make([]outArtifact, 0, len(artifacts))
	for _, a := range artifacts {
		url, _ := s.Objects.PresignedGet(ctx, a.ObjectKey, 15*time.Minute)
		out = append(out, outArtifact{Artifact: a, URL: url})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"job":       job,
		"artifacts": out,
	})
}

// ---- helpers ----

func routePatternFromCtx(ctx context.Context) string {
	if rc := chi.RouteContext(ctx); rc != nil {
		return rc.RoutePattern()
	}
	return ""
}

func clientKey(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	return r.RemoteAddr
}

func resolveKind(explicit, filename string) media.Kind {
	if explicit != "" {
		return media.Kind(strings.ToLower(explicit))
	}
	switch strings.ToLower(ext(filename)) {
	case ".pdf", ".tif", ".tiff":
		return media.KindOCR
	default:
		return media.KindImage
	}
}

// parseParams reads the optional per-job image overrides from the multipart
// form. It returns nil when the client sent none, so the common path keeps the
// job payload (and the worker behaviour) unchanged. Out-of-range or unknown
// values are dropped rather than rejected — the worker clamps to its defaults.
func parseParams(r *http.Request) *media.JobParams {
	p := &media.JobParams{}
	set := false

	if n, ok := formInt(r, "resize_max_dim"); ok && n >= 16 && n <= 8000 {
		p.ResizeMaxDim = n
		set = true
	}
	if n, ok := formInt(r, "thumbnail_size"); ok && n >= 16 && n <= 2000 {
		p.ThumbnailSize = n
		set = true
	}
	switch strings.ToLower(strings.TrimSpace(r.FormValue("resize_format"))) {
	case "jpeg", "png", "webp":
		p.ResizeFormat = strings.ToLower(strings.TrimSpace(r.FormValue("resize_format")))
		set = true
	}
	if n, ok := formInt(r, "quality"); ok && n >= 1 && n <= 100 {
		p.Quality = n
		set = true
	}

	if !set {
		return nil
	}
	return p
}

func formInt(r *http.Request, key string) (int, bool) {
	v := strings.TrimSpace(r.FormValue(key))
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}

func parseOperations(raw string) []media.Operation {
	if strings.TrimSpace(raw) == "" {
		return []media.Operation{media.OpResize, media.OpThumbnail, media.OpWebP}
	}
	var ops []media.Operation
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			ops = append(ops, media.Operation(p))
		}
	}
	return ops
}

func detectContentType(header, filename string) string {
	if header != "" && header != "application/octet-stream" {
		return header
	}
	switch strings.ToLower(ext(filename)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".pdf":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}

func ext(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i:]
	}
	return ""
}

func sanitize(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	if name == "" {
		return "upload.bin"
	}
	return name
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
