package main

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"time"

	"pawprintpublic/internal/config"
	"pawprintpublic/internal/helpers"

	"github.com/justinas/nosurf"
)

// type responseWriter struct {
// 	http.ResponseWriter
// 	buf        *bytes.Buffer
// 	statusCode int
// }

// // WriteHeader captures the status code
// func (w *responseWriter) WriteHeader(statusCode int) {
// 	w.statusCode = statusCode
// }

// // Write writes the data to the buffer instead of the client
// func (w *responseWriter) Write(b []byte) (int, error) {
// 	return w.buf.Write(b)
// }

// // Minify is the middleware function
// func Minify(next http.Handler) http.Handler {
// 	m := minify.New()
// 	m.AddFunc("text/html", html.Minify)

// 	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		// Create a custom responseWriter
// 		rw := &responseWriter{
// 			ResponseWriter: w,
// 			buf:            &bytes.Buffer{},
// 			statusCode:     http.StatusOK,
// 		}

// 		// Call the next handler
// 		next.ServeHTTP(rw, r)

// 		// Get the Content-Type
// 		contentType := rw.Header().Get("Content-Type")
// 		if contentType != "" && !strings.Contains(contentType, "text/html") {
// 			// Not HTML content; write the original response
// 			w.WriteHeader(rw.statusCode)
// 			w.Write(rw.buf.Bytes())
// 			return
// 		}

// 		// Minify the HTML content
// 		minifiedContent, err := m.String("text/html", rw.buf.String())
// 		if err != nil {
// 			// On error, write the original content
// 			w.WriteHeader(rw.statusCode)
// 			w.Write(rw.buf.Bytes())
// 			return
// 		}

// 		// Copy headers
// 		for key, values := range rw.Header() {
// 			for _, value := range values {
// 				w.Header().Add(key, value)
// 			}
// 		}

// 		// Write the minified content
// 		w.Header().Set("Content-Length", strconv.Itoa(len(minifiedContent)))
// 		w.WriteHeader(rw.statusCode)
// 		w.Write([]byte(minifiedContent))
// 	})
// }

// responseWriter wraps http.ResponseWriter and captures the status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code and calls the underlying WriteHeader
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Implement http.Flusher
func (rw *responseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Implement http.Hijacker
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, fmt.Errorf("responseWriter does not implement http.Hijacker")
}

// Implement http.Pusher
func (rw *responseWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := rw.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return fmt.Errorf("responseWriter does not implement http.Pusher")
}

// LogRequest logs each incoming HTTP request
func LogRequest(app *config.AppConfig) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := &responseWriter{ResponseWriter: w, statusCode: 200}
			next.ServeHTTP(ww, r)
			duration := time.Since(start)
			app.InfoLog.Printf(
				"%s - %s %s %s - %d %s",
				r.RemoteAddr,
				r.Method,
				r.RequestURI,
				r.Proto,
				ww.statusCode,
				duration,
			)
		})
	}
}

// NoSurf adds CSRF protection to all POST requests
func NoSurf(next http.Handler) http.Handler {
	csrfHandler := nosurf.New(next)

	csrfHandler.SetBaseCookie(http.Cookie{
		HttpOnly: true,
		Path:     "/",
		Secure:   app.InProduction,
		SameSite: http.SameSiteLaxMode,
	})
	return csrfHandler
}

// SessionLoad loads and saves the session on every request
func SessionLoad(next http.Handler) http.Handler {
	return session.LoadAndSave(next)
}

func Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !helpers.IsAuthenticated(r) {
			session.Put(r.Context(), "error", "Log in first!")
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}
