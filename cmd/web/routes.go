package main

import (
	"net/http"
	"pawprintpublic/internal/config"
	"pawprintpublic/internal/handlers"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
)

func routes(app *config.AppConfig) http.Handler {
	mux := chi.NewRouter()

	mux.Use(middleware.Recoverer)
	mux.Use(NoSurf)
	mux.Use(SessionLoad)
	mux.Use(LogRequest(app))

	mux.Get("/login", handlers.Repo.Login)
	mux.Post("/login", handlers.Repo.PostLogin)
	mux.Get("/logout", handlers.Repo.Logout)

	fileServer := http.FileServer(http.Dir("./static/"))
	mux.Handle("/static/*", http.StripPrefix("/static", fileServer))

	mux.Route("/", func(mux chi.Router) {
		mux.Use(Auth)
		mux.Get("/", handlers.Repo.Home)
		mux.Get("/file-upload", handlers.Repo.FileUploadPage)
		mux.Get("/term-select", handlers.Repo.TermSelectPage)

		mux.Post("/upload", handlers.Repo.UploadHandler)
		mux.Get("/sse", handlers.Repo.SSEHandler)
		mux.Get("/download/{src}", handlers.Repo.DownloadHandler)
		mux.Get("/admin", handlers.Repo.AdminDashboard)

		mux.Get("/admin/users", handlers.Repo.AdminUsers)
		mux.Post("/admin/users/add", handlers.Repo.AdminAddUser)
		mux.Post("/admin/users/edit", handlers.Repo.AdminEditUser)
	})

	return mux
}
