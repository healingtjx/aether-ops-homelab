package templates

import (
	"html/template"
	"io/fs"
	"net/http"
	"strconv"
	"time"

	"aether_ops/internal/middleware"
)

type Renderer struct {
	tmpl *template.Template
}

func New(fsys fs.FS) (*Renderer, error) {
	tmpl, err := template.New("").
		Funcs(template.FuncMap{
			"or": func(def string, v *string) string {
				if v == nil || *v == "" {
					return def
				}
				return *v
			},
			"deref": func(p *string) string {
				if p == nil {
					return ""
				}
				return *p
			},
			"derefInt64": func(p *int64) int64 {
				if p == nil {
					return 0
				}
				return *p
			},
			"formatTime": func(v any) string {
				t := templateTime(v)
				if t == nil {
					return ""
				}
				return t.Format("01-02 15:04")
			},
			"formatTimeFull": func(v any) string {
				t := templateTime(v)
				if t == nil {
					return ""
				}
				return t.Format("2006-01-02 15:04:05")
			},
			"truncate": func(max int, s string) string {
				if len(s) <= max {
					return s
				}
				return s[:max] + "..."
			},
			"formatBytes": func(p *int64) string {
				if p == nil {
					return ""
				}
				value := *p
				units := []string{"B", "KB", "MB", "GB", "TB"}
				size := float64(value)
				unit := 0
				for size >= 1024 && unit < len(units)-1 {
					size = size / 1024
					unit++
				}
				if unit == 0 {
					return strconv.FormatInt(value, 10) + " " + units[unit]
				}
				return strconv.FormatFloat(size, 'f', 1, 64) + " " + units[unit]
			},
			"dict": func(values ...any) map[string]any {
				dict := map[string]any{}
				for i := 0; i+1 < len(values); i += 2 {
					key, ok := values[i].(string)
					if !ok {
						continue
					}
					dict[key] = values[i+1]
				}
				return dict
			},
			"int64zero": func() int64 {
				return 0
			},
		}).
		ParseFS(fsys, "web/templates/*.html", "web/templates/**/*.html")
	if err != nil {
		return nil, err
	}
	return &Renderer{tmpl: tmpl}, nil
}

func templateTime(v any) *time.Time {
	switch t := v.(type) {
	case time.Time:
		return &t
	case *time.Time:
		return t
	default:
		return nil
	}
}

func (r *Renderer) Page(w http.ResponseWriter, req *http.Request, name string, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	if u := middleware.CurrentUser(req.Context()); u != nil {
		data["CurrentUser"] = u.Username
	} else {
		data["CurrentUser"] = ""
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := r.tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (r *Renderer) Partial(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := r.tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
