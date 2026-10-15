package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"aether_ops/internal/models"
)

type contextKey string

const UserKey contextKey = "user"
const defaultLoginPath = "/bs"

func CurrentUser(ctx context.Context) *models.User {
	u, _ := ctx.Value(UserKey).(*models.User)
	return u
}

func Authenticate(db *pgxpool.Pool, loginPath string) func(http.Handler) http.Handler {
	loginPath = normalizeLoginPath(loginPath)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("session_token")
			if err != nil {
				unauthorized(w, r, loginPath)
				return
			}

			session, err := models.GetSessionByToken(r.Context(), db, cookie.Value)
			if err != nil {
				clearSessionCookie(w)
				unauthorized(w, r, loginPath)
				return
			}

			user, err := models.GetUserByID(r.Context(), db, session.UserID)
			if err != nil {
				models.DeleteSession(r.Context(), db, session.Token)
				clearSessionCookie(w)
				unauthorized(w, r, loginPath)
				return
			}

			ctx := context.WithValue(r.Context(), UserKey, &user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func unauthorized(w http.ResponseWriter, r *http.Request, loginPath string) {
	loginPath = normalizeLoginPath(loginPath)
	w.Header().Set("X-Login-Path", loginPath)
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", loginPath)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write([]byte("event: auth-expired\ndata: " + loginPath + "\n\n"))
		return
	}
	if r.Header.Get("X-Requested-With") != "" {
		w.Header().Set("Location", loginPath)
		http.Error(w, "登录信息已过期", http.StatusUnauthorized)
		return
	}
	http.Redirect(w, r, loginPath, http.StatusSeeOther)
}

func normalizeLoginPath(loginPath string) string {
	loginPath = strings.TrimSpace(loginPath)
	if loginPath == "" {
		return defaultLoginPath
	}
	if !strings.HasPrefix(loginPath, "/") {
		return "/" + loginPath
	}
	return loginPath
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:   "session_token",
		Value:  "",
		MaxAge: -1,
		Path:   "/",
	})
}
