package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"aether_ops/internal/middleware"
	"aether_ops/internal/models"
	"aether_ops/internal/templates"
)

type AuthHandler struct {
	db        *pgxpool.Pool
	rdr       *templates.Renderer
	loginPath string
}

func NewAuthHandler(db *pgxpool.Pool, rdr *templates.Renderer, loginPath string) *AuthHandler {
	return &AuthHandler{db: db, rdr: rdr, loginPath: loginPath}
}

func (h *AuthHandler) Register(r chi.Router) {
	r.Get(h.loginPath, h.LoginPage)
	r.Post(h.loginPath, h.Login)
	r.Post("/logout", h.Logout)
}

func (h *AuthHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	errorMsg := ""
	if r.URL.Query().Get("error") == "1" {
		errorMsg = "用户名或密码错误"
	}
	h.rdr.Page(w, r, "login_page", map[string]any{
		"Error":     errorMsg,
		"LoginPath": h.loginPath,
	})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")

	user, err := models.GetUserByUsername(r.Context(), h.db, username)
	if err != nil {
		http.Redirect(w, r, h.loginPath+"?error=1", http.StatusSeeOther)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		http.Redirect(w, r, h.loginPath+"?error=1", http.StatusSeeOther)
		return
	}

	session, err := models.CreateSession(r.Context(), h.db, user.ID)
	if err != nil {
		http.Redirect(w, r, h.loginPath+"?error=1", http.StatusSeeOther)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    session.Token,
		Path:     "/",
		MaxAge:   86400,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("session_token"); err == nil {
		models.DeleteSession(r.Context(), h.db, cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:   "session_token",
		Value:  "",
		MaxAge: -1,
		Path:   "/",
	})

	http.Redirect(w, r, h.loginPath, http.StatusSeeOther)
}

// CurrentUserID extracts the current user's ID from the request context.
// Returns 0 if no user is authenticated.
func CurrentUserID(r *http.Request) int64 {
	if u := middleware.CurrentUser(r.Context()); u != nil {
		return u.ID
	}
	return 0
}
