package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/vhysxl/relay/bootstrap/app"
	"github.com/vhysxl/relay/internal/api"
)

func Listen(ctx context.Context, app *app.Application) error {
	mux := http.NewServeMux()

	api.RegisterRoutes(mux, app)

	srv := &http.Server{
		Addr:    app.Config.Port,
		Handler: mux,
	}

	serverErr := make(chan error, 1)
	go func() {
		fmt.Printf("Server is running and listening on port %s\n", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		fmt.Println("Shutting down HTTP server gracefully...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
