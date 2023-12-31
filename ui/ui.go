package ui

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/guettli/watchall/config"
)

func RunUIWithContext(ctx context.Context, args config.Arguments, kubeconfig clientcmd.ClientConfig) error {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		page(
			r.URL.Query().Get("ns"),
		).Render(r.Context(), w)
	})
	http.Handle("/static/", http.StripPrefix("/static/", setContentTypeMiddleware(
		http.FileServer(http.Dir("./static")))))

	fmt.Println("Listening on http://localhost:3000/")
	return http.ListenAndServe(":3000", nil)
}

type contentEncodingReponseWriter struct {
	wrapped http.ResponseWriter
	request *http.Request
}

func (w *contentEncodingReponseWriter) Header() http.Header {
	return w.wrapped.Header()
}

func (w *contentEncodingReponseWriter) Write(b []byte) (int, error) {
	return w.wrapped.Write(b)
}

func (w *contentEncodingReponseWriter) WriteHeader(statusCode int) {
	w.rewriteHeader(statusCode)
	w.wrapped.WriteHeader(statusCode)
}

func (w *contentEncodingReponseWriter) rewriteHeader(statusCode int) {
	r := w.request
	if statusCode != 200 {
		return
	}
	if !strings.HasSuffix(r.URL.Path, ".gz") {
		return
	}
	if w.Header().Get("Content-Type") != "application/gzip" {
		return
	}
	ext := filepath.Ext(strings.TrimSuffix(r.URL.Path, ".gz"))
	if ext == "" {
		return
	}
	w.Header().Set("Content-Encoding", "gzip")
	switch ext {
	case ".css":
		w.Header().Set("Content-Type", "text/css")
	case ".js":
		w.Header().Set("Content-Type", "text/javascript")
	}
}

func setContentTypeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&contentEncodingReponseWriter{wrapped: w, request: r}, r)
	})
}
