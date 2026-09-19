package server

import (
	"fmt"
	"net/http"
)

func Listen(port string) error {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("Task Queue Server Online")
	})

	fmt.Printf("Server is running and listening on port %s\n", port)

	err := http.ListenAndServe(port, mux)
	if err != nil {
		return err
	}

	return nil
}
