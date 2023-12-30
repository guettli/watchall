package ui

import (
	"context"
	"fmt"
	"net/http"

	"github.com/a-h/templ"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/guettli/watchall/config"
)

func RunUIWithContext(ctx context.Context, args config.Arguments, kubeconfig clientcmd.ClientConfig) error {
	http.Handle("/", templ.Handler(page()))

	fmt.Println("Listening on :3000")
	return http.ListenAndServe(":3000", nil)
}
