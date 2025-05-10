package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"publisher_subscriber/internal/server/grpc"
	"publisher_subscriber/internal/server/grpc/config"
	"publisher_subscriber/internal/subpub"
)

func main() {
	cfg, err := config.Load("config/config.yaml")
	if err != nil {
		log.Fatal("Error loading config: ", err)
	}

	pubSub := subpub.NewSubPub()

	ctx := context.Background()
	defer pubSub.Close(ctx)

	srv := grpc.New(cfg, pubSub)

	go func() {
		err := srv.Start()
		if err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutdown Server ...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	srv.Stop()
	log.Println("Server stopped")
}
