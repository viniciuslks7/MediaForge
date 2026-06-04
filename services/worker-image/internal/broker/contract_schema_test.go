package broker

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

const schemaDir = "../../../../specs/schemas"

func compile(t *testing.T, file string) *jsonschema.Schema {
	t.Helper()
	sch, err := jsonschema.Compile(filepath.ToSlash(filepath.Join(schemaDir, file)))
	if err != nil {
		t.Fatalf("compile %s: %v", file, err)
	}
	return sch
}

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

// TestJobDecodesFromContractSample proves the worker can consume a payload shaped
// exactly like the canonical Job contract (the schema's required fields).
func TestJobDecodesFromContractSample(t *testing.T) {
	wire := []byte(`{
		"job_id": "9c1f0c4a-2b3d-4e5f-8a9b-0c1d2e3f4a5b",
		"kind": "image",
		"status": "pending",
		"source_key": "uploads/9c1f/sample.png",
		"source_mime": "image/png",
		"size_bytes": 12345,
		"operations": ["resize", "thumbnail", "webp"]
	}`)

	// Must satisfy the contract...
	sch := compile(t, "job.schema.json")
	var inst any
	if err := json.Unmarshal(wire, &inst); err != nil {
		t.Fatalf("unmarshal wire: %v", err)
	}
	if err := sch.Validate(inst); err != nil {
		t.Fatalf("sample violates contract: %v", err)
	}

	// ...and the worker's own struct must decode it without loss.
	var job Job
	if err := json.Unmarshal(wire, &job); err != nil {
		t.Fatalf("worker decode: %v", err)
	}
	if job.ID == "" || job.Kind != "image" || len(job.Operations) != 3 {
		t.Fatalf("worker decoded job incorrectly: %+v", job)
	}
}

// TestEventConformsToContract proves the events the worker emits are valid.
func TestEventConformsToContract(t *testing.T) {
	sch := compile(t, "event.schema.json")
	event := Event{
		JobID:     "9c1f0c4a-2b3d-4e5f-8a9b-0c1d2e3f4a5b",
		Kind:      "image",
		Status:    "processing",
		Message:   "transforming",
		Progress:  40,
		Artifact:  "thumbnail",
		Timestamp: time.Now().UnixMilli(),
	}
	if err := sch.Validate(asInstance(t, event)); err != nil {
		t.Fatalf("emitted event violates contract: %v", err)
	}
}
