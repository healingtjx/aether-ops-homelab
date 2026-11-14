package templates

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"aether_ops/internal/models"
)

func TestOperationLogDeleteButton(t *testing.T) {
	rdr, err := New(os.DirFS("../.."))
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"running", "success"} {
		t.Run(status, func(t *testing.T) {
			w := httptest.NewRecorder()
			rdr.Partial(w, "operation_log_row", models.OperationLog{ID: 42, OperationType: "artifact_build", Status: status})
			body := w.Body.String()
			if w.Code != 200 || !strings.Contains(body, `hx-delete="/operation-logs/42"`) {
				t.Fatalf("missing delete action: %s", body)
			}
			if strings.Contains(body, "不会终止后台进程") != (status == "running") {
				t.Fatalf("incorrect confirmation for %s: %s", status, body)
			}
		})
	}
}
