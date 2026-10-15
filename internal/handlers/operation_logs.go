package handlers

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"aether_ops/internal/models"
	"aether_ops/internal/templates"
)

const pageSize = 20

type OperationLogHandler struct {
	db  *pgxpool.Pool
	rdr *templates.Renderer
}

func NewOperationLogHandler(db *pgxpool.Pool, rdr *templates.Renderer) *OperationLogHandler {
	return &OperationLogHandler{db: db, rdr: rdr}
}

func (h *OperationLogHandler) Register(r chi.Router) {
	r.Get("/operation-logs", h.List)
	r.Get("/operation-logs/rows", h.ListRows)
	r.Get("/applications/{id}/logs", h.ListByApp)
}

func (h *OperationLogHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	projectID, _ := strconv.ParseInt(r.URL.Query().Get("project_id"), 10, 64)
	appID, _ := strconv.ParseInt(r.URL.Query().Get("application_id"), 10, 64)
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	page = max(page, 1)

	data, err := h.buildListData(ctx, projectID, appID, page, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.rdr.Page(w, r, "operation_logs_page", data)
}

func (h *OperationLogHandler) ListRows(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	projectID, _ := strconv.ParseInt(r.URL.Query().Get("project_id"), 10, 64)
	appID, _ := strconv.ParseInt(r.URL.Query().Get("application_id"), 10, 64)
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	page = max(page, 1)

	data, err := h.buildListData(ctx, projectID, appID, page, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.rdr.Partial(w, "operation_logs_results", data)
}

func (h *OperationLogHandler) ListByApp(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	appID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	app, err := models.GetApplication(ctx, h.db, appID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	page = max(page, 1)

	data, err := h.buildListData(ctx, 0, appID, page, app.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data["BackURL"] = "/applications"
	h.rdr.Page(w, r, "operation_logs_page", data)
}

type pagination struct {
	Page       int
	TotalPages int
	HasPrev    bool
	HasNext    bool
	PrevPage   int
	NextPage   int
}

func (h *OperationLogHandler) buildListData(ctx context.Context, projectID, appID int64, page int, titleSuffix string) (map[string]any, error) {
	total, err := models.CountOperationLogs(ctx, h.db, projectID, appID)
	if err != nil {
		return nil, err
	}

	totalPages := max(int(math.Ceil(float64(total)/float64(pageSize))), 1)
	page = min(max(page, 1), totalPages)

	logs, err := models.ListOperationLogs(ctx, h.db, projectID, appID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, err
	}

	projects, err := models.ListProjects(ctx, h.db)
	if err != nil {
		return nil, err
	}

	applications, err := models.ListApplications(ctx, h.db)
	if err != nil {
		return nil, err
	}

	pag := pagination{
		Page:       page,
		TotalPages: totalPages,
		HasPrev:    page > 1,
		HasNext:    page < totalPages,
		PrevPage:   page - 1,
		NextPage:   page + 1,
	}

	return map[string]any{
		"Logs":                  logs,
		"Projects":              projects,
		"Applications":          applications,
		"SelectedProjectID":     projectID,
		"SelectedApplicationID": appID,
		"TitleSuffix":           titleSuffix,
		"Pagination":            pag,
	}, nil
}
