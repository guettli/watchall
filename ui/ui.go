package ui

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"

	_ "net/http"
	_ "net/http/pprof" // see http://localhost:1234/debug/pprof

	"github.com/guettli/contentencoding"
	"github.com/guettli/watchall/config"
	"github.com/guettli/watchall/dbstuff"
)

func errorHandler(w http.ResponseWriter, r *http.Request, status int) {
	w.WriteHeader(status)
	if status == http.StatusNotFound {
		fmt.Fprint(w, "custom 404")
	}
}

func RunUIWithContext(ctx context.Context, args config.Arguments, db *sql.DB) {
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
		rows, err := db.Query("select * from res where name like ?", "%"+query+"%")
		if err != nil {
			panic(err) // todo
		}
		defer rows.Close()
		var resources []dbstuff.Resource
		for rows.Next() {
			res, err := dbstuff.ResourceNewFromRow(rows)
			if err != nil {
				panic(err) // todo
			}
			resources = append(resources, res)
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
