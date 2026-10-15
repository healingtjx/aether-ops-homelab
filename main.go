package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/crypto/bcrypt"

	"aether_ops/internal/config"
	"aether_ops/internal/db"
	"aether_ops/internal/handlers"
	mw "aether_ops/internal/middleware"
	"aether_ops/internal/models"
	"aether_ops/internal/scheduler"
	tpl "aether_ops/internal/templates"
)

const loginPath = "/bs"

//go:embed web/templates/*
var templateFS embed.FS

func main() {
	cfg := config.Load()

	if len(os.Args) > 1 && os.Args[1] == "create-user" {
		if len(os.Args) < 4 {
			fmt.Fprintf(os.Stderr, "Usage: %s create-user <username> <password>\n", os.Args[0])
			os.Exit(1)
		}
		pool, err := db.Connect(cfg.Database)
		if err != nil {
			log.Fatalf("database connect: %v", err)
		}
		defer pool.Close()

		username := os.Args[2]
		hash, err := bcrypt.GenerateFromPassword([]byte(os.Args[3]), bcrypt.DefaultCost)
		if err != nil {
			log.Fatalf("hash password: %v", err)
		}
		if _, err := models.CreateUser(context.Background(), pool, username, string(hash)); err != nil {
			log.Fatalf("create user: %v", err)
		}
		fmt.Printf("User %q created successfully\n", username)
		return
	}

	pool, err := db.Connect(cfg.Database)
	if err != nil {
		log.Fatalf("database connect: %v", err)
	}
	defer pool.Close()

	renderer, err := tpl.New(templateFS)
	if err != nil {
		log.Fatalf("template parse: %v", err)
	}

	cleanupCron := scheduler.Start(pool)
	defer cleanupCron.Stop()

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	handlers.NewAuthHandler(pool, renderer, loginPath).Register(r)

	r.Group(func(r chi.Router) {
		r.Use(mw.Authenticate(pool, loginPath))

		handlers.NewProjectHandler(pool, renderer).Register(r)
		handlers.NewApplicationHandler(pool, renderer, cfg.WorkDir.ApplicationPath).Register(r)
		handlers.NewOperationLogHandler(pool, renderer).Register(r)
		handlers.NewReleaseScriptHandler(pool, renderer).Register(r)
		handlers.NewCIConfigHandler(pool, renderer, cfg.WorkDir.CIPath).Register(r)
		handlers.NewArtifactHandler(pool, renderer).Register(r)
		handlers.NewArtifactBuildHandler(pool, renderer, cfg.WorkDir.CIPath).Register(r)
		handlers.NewDockerDeploymentHandler(pool, renderer).Register(r)
		handlers.NewGlobalConfigHandler(pool, renderer).Register(r)

		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/projects", http.StatusFound)
		})
	})

	log.Printf("listening on :%s", cfg.Server.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Server.Port, r))
}
