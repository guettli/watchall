package ui

import (
	"context"
	"fmt"
	"log"
	"net/http"

	_ "net/http"
	_ "net/http/pprof" // see http://localhost:1234/debug/pprof

	"github.com/guettli/contentencoding"
	"github.com/guettli/watchall/config"
	"github.com/guettli/watchall/dbstuff"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

func errorHandler(w http.ResponseWriter, r *http.Request, status int) {
	w.WriteHeader(status)
	if status == http.StatusNotFound {
		fmt.Fprint(w, "custom 404")
	}
}

func RunUIWithContext(ctx context.Context, args config.Arguments) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			errorHandler(w, r, http.StatusNotFound)
			return
		}
		query := r.URL.Query().Get("query")
		page(
			query,
			fmt.Sprintf("/hxRows?page=0"+"&query="+query),
		).Render(r.Context(), w)
	})

	http.HandleFunc("/hxRows", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		var resources []dbstuff.Resource
		err := dbstuff.Query(ctx, args.Pool, "select * from res where name like ?", &sqlitex.ExecOptions{
			Args: []any{"%" + query + "%"},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				res := dbstuff.ResourceNewFromRow(stmt)
				resources = append(resources, res)
				return nil
			},
		})
		if err != nil {
			panic(err) // todo
		}

		hxRows(resources).Render(r.Context(), w)
	})
	http.Handle("/static/", http.StripPrefix("/static/",
		contentencoding.FileServer(http.Dir("./static"))))

	fmt.Println("Listening on http://localhost:3000/")

	srv := &http.Server{Addr: ":3000"}
	// always returns error. ErrServerClosed on graceful close
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		// unexpected error. port in use?
		log.Fatalf("ListenAndServe(): %v", err)
	}
}
