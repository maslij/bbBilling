package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"

	"brinkbyte-billing-server/handlers"
	"brinkbyte-billing-server/middleware"
	"brinkbyte-billing-server/storage"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize storage based on environment
	var store handlers.Storage
	var err error

	usePostgres := os.Getenv("USE_POSTGRES") != "false"
	if usePostgres {
		// Try PostgreSQL storage
		config := storage.LoadPostgresConfigFromEnv()
		pgStore, pgErr := storage.NewPostgresStorage(ctx, config)
		if pgErr != nil {
			log.Printf("⚠️  PostgreSQL unavailable (%v), falling back to in-memory storage", pgErr)
			store = storage.NewInMemoryStorage()
		} else {
			store = pgStore
			log.Println("✅ Using PostgreSQL storage")
		}
	} else {
		store = storage.NewInMemoryStorage()
		log.Println("📦 Using in-memory storage (USE_POSTGRES=false)")
	}

	// Initialize handlers
	handler := handlers.NewHandler(store)

	// Setup router
	r := mux.NewRouter()

	// Apply global middleware
	r.Use(middleware.Logging)
	r.Use(middleware.CORS)

	// Handle OPTIONS requests for CORS preflight (must be before other routes)
	r.Methods("OPTIONS").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.WriteHeader(http.StatusOK)
	})

	// API routes - Billing endpoints (new)
	api := r.PathPrefix("/api/v1").Subrouter()

	// Apply auth middleware to API routes (optional based on env)
	if os.Getenv("REQUIRE_AUTH") == "true" {
		api.Use(middleware.AuthMiddleware(store))
	}

	// License & Subscription endpoints (GET)
	// NOTE: More specific routes must come BEFORE parameterized routes
	api.HandleFunc("/billing/growth-packs/available", handler.GetAvailableGrowthPacks).Methods("GET")
	api.HandleFunc("/billing/pricing", handler.GetPricingConfig).Methods("GET")
	api.HandleFunc("/billing/license/{tenantId}", handler.GetLicenseStatus).Methods("GET")
	api.HandleFunc("/billing/license/{tenantId}/revoke", handler.RevokeLicense).Methods("POST")
	api.HandleFunc("/billing/subscription/{tenantId}", handler.GetSubscription).Methods("GET")
	api.HandleFunc("/billing/growth-packs/{tenantId}", handler.GetEnabledGrowthPacks).Methods("GET")
	api.HandleFunc("/billing/usage/{tenantId}", handler.GetUsageSummary).Methods("GET")
	api.HandleFunc("/billing/validate", handler.ValidateCameraLicense).Methods("POST")

	// Legacy endpoints (POST) - for backwards compatibility with C++ client
	api.HandleFunc("/licenses/validate", handler.ValidateLicense).Methods("POST")
	api.HandleFunc("/entitlements/check", handler.CheckEntitlement).Methods("POST")
	api.HandleFunc("/usage/batch", handler.ReportUsageBatch).Methods("POST")
	api.HandleFunc("/heartbeat", handler.Heartbeat).Methods("POST")

	// Admin routes (protected)
	admin := r.PathPrefix("/api/v1/admin").Subrouter()
	if os.Getenv("REQUIRE_ADMIN_AUTH") == "true" {
		admin.Use(middleware.AdminAuthMiddleware)
	}
	admin.HandleFunc("/tenants", handler.CreateTenant).Methods("POST")
	admin.HandleFunc("/tenants/{id}", handler.UpdateTenant).Methods("PUT")
	admin.HandleFunc("/tenants/{id}", handler.GetTenantAdmin).Methods("GET")
	admin.HandleFunc("/subscriptions", handler.CreateSubscription).Methods("POST")
	admin.HandleFunc("/subscriptions/{id}", handler.UpdateSubscription).Methods("PUT")
	admin.HandleFunc("/subscriptions/{tenantId}/growth-packs", handler.ManageGrowthPacks).Methods("PUT")

	// Public routes
	r.HandleFunc("/health", handler.HealthCheck).Methods("GET")
	r.HandleFunc("/stats", handler.GetStats).Methods("GET")

	// Start server
	port := getEnvOrDefault("PORT", "8081")
	addr := ":" + port

	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		log.Println("🛑 Shutting down server...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("Error during shutdown: %v", err)
		}
		cancel()
	}()

	log.Printf("🚀 BrinkByte Vision Billing Server starting on port %s", port)
	log.Printf("📊 New Billing API Endpoints:")
	log.Printf("   GET  http://localhost%s/api/v1/billing/license/{tenantId}", addr)
	log.Printf("   GET  http://localhost%s/api/v1/billing/subscription/{tenantId}", addr)
	log.Printf("   GET  http://localhost%s/api/v1/billing/growth-packs/{tenantId}", addr)
	log.Printf("   GET  http://localhost%s/api/v1/billing/growth-packs/available", addr)
	log.Printf("   GET  http://localhost%s/api/v1/billing/pricing", addr)
	log.Printf("   GET  http://localhost%s/api/v1/billing/usage/{tenantId}", addr)
	log.Printf("   POST http://localhost%s/api/v1/billing/validate", addr)
	log.Printf("")
	log.Printf("📊 Legacy API Endpoints (C++ client):")
	log.Printf("   POST http://localhost%s/api/v1/licenses/validate", addr)
	log.Printf("   POST http://localhost%s/api/v1/entitlements/check", addr)
	log.Printf("   POST http://localhost%s/api/v1/usage/batch", addr)
	log.Printf("   POST http://localhost%s/api/v1/heartbeat", addr)
	log.Printf("")
	log.Printf("🔧 Admin Endpoints:")
	log.Printf("   POST http://localhost%s/api/v1/admin/tenants", addr)
	log.Printf("   PUT  http://localhost%s/api/v1/admin/tenants/{id}", addr)
	log.Printf("   POST http://localhost%s/api/v1/admin/subscriptions", addr)
	log.Printf("   PUT  http://localhost%s/api/v1/admin/subscriptions/{tenantId}/growth-packs", addr)
	log.Printf("")
	log.Printf("📊 Admin Endpoints:")
	log.Printf("   GET  http://localhost%s/health", addr)
	log.Printf("   GET  http://localhost%s/stats", addr)
	log.Printf("")
	log.Printf("✨ Server ready to accept connections!")

	if err = srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}

	log.Println("👋 Server stopped")
}

func getEnvOrDefault(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}
