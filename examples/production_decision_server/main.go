package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/oarkflow/condition"
	_ "github.com/oarkflow/condition/bcl"
)

func main() {
	addr := getenv("ADDR", ":8080")
	dbPath := getenv("DECISION_SQLITE_PATH", "/data/decisions.db")
	adminToken := os.Getenv("DECISION_ADMIN_TOKEN")
	if adminToken == "" {
		log.Fatal("DECISION_ADMIN_TOKEN is required")
	}
	store, err := condition.NewSQLiteDecisionStore(dbPath)
	if err != nil {
		log.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()

	authzFile := getenv("DECISION_AUTHZ_FILE", "examples/production_decision_server/production.authz")
	authorizer, err := condition.NewAuthzAuthorizerFromFile(context.Background(), authzFile)
	if err != nil {
		log.Fatalf("load authz policy file %s: %v", authzFile, err)
	}
	decisionServer, err := condition.NewProductionDecisionServer(condition.ProductionDecisionServerConfig{
		Store:      store,
		Authorizer: authorizer,
		Config:     condition.DefaultProductionDecisionServerConfig(),
		Logger:     stdLogger{},
	})
	if err != nil {
		log.Fatalf("production server config: %v", err)
	}

	if path := os.Getenv("DECISION_PACKAGE"); path != "" {
		pkg, err := condition.LoadDecisionPackageFile(context.Background(), condition.DecisionPackageFile{Path: path})
		if err != nil {
			log.Fatalf("load package: %v", err)
		}
		if _, err := condition.ValidateProductionDecisionPackage(context.Background(), pkg); err != nil {
			log.Fatalf("validate package: %v", err)
		}
		if _, err := decisionServer.PublishPackage(context.Background(), pkg); err != nil {
			log.Fatalf("publish package: %v", err)
		}
	}

	mux := http.NewServeMux()
	mux.Handle("/v1/", withPrincipal(adminToken, decisionServer))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if _, err := store.ListPackages(r.Context()); err != nil {
			writeError(w, http.StatusServiceUnavailable, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/audit/verify", func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") != adminToken {
			writeError(w, http.StatusUnauthorized, errors.New("admin bearer token is required"))
			return
		}
		envelopes, err := store.ListAuditEnvelopes(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if err := condition.VerifyAuditChain(envelopes); err != nil {
			writeError(w, http.StatusConflict, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "envelopes": len(envelopes)})
	})

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()
	log.Printf("production decision server listening on %s sqlite=%s authz=%s", addr, dbPath, authzFile)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func withPrincipal(adminToken string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == adminToken {
			p := condition.Principal{ID: "admin", Tenant: getenv("DECISION_TENANT", "default"), Roles: []condition.Role{"manager"}}
			next.ServeHTTP(w, r.WithContext(condition.WithPrincipal(r.Context(), p)))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

type stdLogger struct{}

func (stdLogger) Log(_ context.Context, level, message string, fields map[string]any) {
	log.Printf("%s %s %v", level, message, fields)
}
