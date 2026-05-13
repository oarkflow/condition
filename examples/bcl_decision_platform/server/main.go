package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/oarkflow/condition"
	"github.com/oarkflow/condition/bcl"
)

func main() {
	addr := getenv("ADDR", ":8080")
	store := condition.NewMemoryDecisionStore()
	server := condition.NewDecisionServer(
		condition.WithDecisionServerStore(store),
		condition.WithDecisionServerConfig(condition.DecisionServerConfig{
			MaxBodyBytes:       10 << 20,
			RequestTimeout:     15 * time.Second,
			RequireContentType: true,
		}),
	)
	if path := os.Getenv("DECISION_PACKAGE"); path != "" {
		pkg, err := condition.LoadDecisionPackage(context.Background(), condition.FileSource{Path: path}, bcl.Decoder())
		if err != nil {
			log.Fatalf("load package: %v", err)
		}
		if _, err := server.PublishPackage(context.Background(), pkg); err != nil {
			log.Fatalf("publish package: %v", err)
		}
	}
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           server,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("decision server listening on %s", addr)
	log.Fatal(httpServer.ListenAndServe())
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
