package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestDeleteOperationLogRejectsInvalidID(t *testing.T) {
	h := NewOperationLogHandler(nil, nil)
	r := chi.NewRouter()
	h.Register(r)
	for _, id := range []string{"bad", "0", "-1", "9223372036854775808"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/operation-logs/"+id, nil))
		if w.Code != http.StatusBadRequest {
			t.Errorf("id %q: status=%d, want 400", id, w.Code)
		}
	}
}
