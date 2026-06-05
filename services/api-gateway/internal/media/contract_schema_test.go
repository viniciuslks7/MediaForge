package media

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// schemaDir locates the canonical JSON Schemas at the repo root, relative to
// this package. Contract tests validate that what this service puts on the wire
// conforms to the single source of truth in specs/schemas.
const schemaDir = "../../../../specs/schemas"

func compile(t *testing.T, file string) *jsonschema.Schema {
	t.Helper()
	sch, err := jsonschema.Compile(filepath.ToSlash(filepath.Join(schemaDir, file)))
	if err != nil {
		t.Fatalf("compile %s: %v", file, err)
	}
	return sch
}

// asInstance marshals v and unmarshals it into the generic shape the validator
// expects — exactly the round-trip a consumer performs off the wire.
func asInstance(t *testing.T, v any) any {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var inst any
	if err := json.Unmarshal(data, &inst); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return inst
}

func TestJobConformsToContract(t *testing.T) {
	sch := compile(t, "job.schema.json")

	// A job exactly as the gateway publishes it on media.jobs.
	job := Job{
		ID:         "9c1f0c4a-2b3d-4e5f-8a9b-0c1d2e3f4a5b",
		Kind:       KindImage,
		Status:     StatusPending,
		SourceKey:  "uploads/9c1f/sample.png",
		SourceMIME: "image/png",
		SizeBytes:  12345,
		Operations: []Operation{OpResize, OpThumbnail, OpWebP},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := sch.Validate(asInstance(t, job)); err != nil {
		t.Fatalf("published job violates contract: %v", err)
	}
}

func TestEventConformsToContract(t *testing.T) {
	sch := compile(t, "event.schema.json")

	event := Event{
		JobID:     "9c1f0c4a-2b3d-4e5f-8a9b-0c1d2e3f4a5b",
		Kind:      KindOCR,
		Status:    StatusCompleted,
		Message:   "done",
		Progress:  100,
		Timestamp: time.Now().UnixMilli(),
	}
	if err := sch.Validate(asInstance(t, event)); err != nil {
		t.Fatalf("emitted event violates contract: %v", err)
	}
}

func TestJobWithParamsConformsToContract(t *testing.T) {
	sch := compile(t, "job.schema.json")

	// A job carrying optional per-job image params must still satisfy the
	// contract — proving the params extension is additive.
	job := Job{
		ID:         "9c1f0c4a-2b3d-4e5f-8a9b-0c1d2e3f4a5b",
		Kind:       KindImage,
		Status:     StatusPending,
		SourceKey:  "uploads/9c1f/sample.png",
		SourceMIME: "image/png",
		SizeBytes:  12345,
		Operations: []Operation{OpResize, OpThumbnail},
		Params:     &JobParams{ResizeMaxDim: 800, ThumbnailSize: 128, ResizeFormat: "webp", Quality: 90},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := sch.Validate(asInstance(t, job)); err != nil {
		t.Fatalf("job with params violates contract: %v", err)
	}
}

func TestInvalidJobRejectedByContract(t *testing.T) {
	sch := compile(t, "job.schema.json")

	// Unknown kind must be rejected — proves the contract actually constrains.
	bad := map[string]any{
		"job_id":     "9c1f0c4a-2b3d-4e5f-8a9b-0c1d2e3f4a5b",
		"kind":       "video",
		"status":     "pending",
		"source_key": "uploads/x",
	}
	if err := sch.Validate(bad); err == nil {
		t.Fatal("expected contract violation for unknown kind, got none")
	}
}
