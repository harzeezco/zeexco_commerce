package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/harzeezco/zeexco_commerce/internal/app"
	"github.com/harzeezco/zeexco_commerce/internal/config"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	cfg := config.Load()
	application, err := app.New(ctx)
	if err != nil {
		log.Fatalf("initialize application: %v", err)
	}
	defer application.Close()
	server := &http.Server{
		Addr:              net.JoinHostPort("", cfg.Port),
		Handler:           application.Handler(),
		ReadHeaderTimeout: mustDuration("HTTP_READ_HEADER_TIMEOUT", cfg.HTTPReadHeaderTimeout),
		ReadTimeout:       mustDuration("HTTP_READ_TIMEOUT", cfg.HTTPReadTimeout),
		WriteTimeout:      mustDuration("HTTP_WRITE_TIMEOUT", cfg.HTTPWriteTimeout),
		IdleTimeout:       mustDuration("HTTP_IDLE_TIMEOUT", cfg.HTTPIdleTimeout),
	}

	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("Zeexco API listening on %s", server.Addr)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serve API: %v", err)
		}
	case <-ctx.Done():
		shutdownTimeout := mustDuration("HTTP_SHUTDOWN_TIMEOUT", cfg.HTTPShutdownTimeout)
		shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			_ = server.Close()
			log.Printf("API forced shutdown: %v", err)
		}
	}
}

func mustDuration(name, value string) time.Duration {
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		log.Fatalf("%s must be a positive duration", name)
	}
	return duration
}
