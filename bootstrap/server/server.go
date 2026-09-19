package server

import (
	"database/sql"
	"fmt"
	"net/http"
	"task/queue/internal/api"
)

func Listen(port string, db *sql.DB) error {
	mux := http.NewServeMux()

	api.RegisterRoutes(mux, db)

	fmt.Printf("Server is running and listening on port %s\n", port)

	err := http.ListenAndServe(port, mux)
	if err != nil {
		return err
	}

	return nil
}
