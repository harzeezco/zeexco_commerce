package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/harzeezco/zeexco_commerce/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	worker, err := app.NewWorker(ctx)
	if err != nil {
		log.Fatalf("initialize background worker: %v", err)
	}
	log.Print("Zeexco background worker started")
	<-ctx.Done()
	worker.Close()
}
