package ui

import (
	"context"
	"fmt"
	"net/http"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/guettli/contentencoding"
	"github.com/guettli/watchall/config"
)

func RunUIWithContext(ctx context.Context, args config.Arguments, kubeconfig clientcmd.ClientConfig) error {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		page(
			r.URL.Query().Get("query"),
		).Render(r.Context(), w)
	})
	http.Handle("/static/", http.StripPrefix("/static/",
		contentencoding.FileServer(http.Dir("./static"))))

	fmt.Println("Listening on http://localhost:3000/")
	return http.ListenAndServe(":3000", nil)
}
