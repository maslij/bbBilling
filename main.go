package main

import (
	"log"
	"net/http"

	"github.com/gorilla/mux"
	
	"brinkbyte-billing-server/handlers"
	"brinkbyte-billing-server/middleware"
	"brinkbyte-billing-server/storage"
)

func main() {
	// Initialize storage
	store := storage.NewInMemoryStorage()
	
	// Initialize handlers
	handler := handlers.NewHandler(store)
	
	// Setup router
	r := mux.NewRouter()
	
	// Apply middleware
	r.Use(middleware.Logging)
	r.Use(middleware.CORS)
	
	// API routes - Billing endpoints (new)
	api := r.PathPrefix("/api/v1").Subrouter()
	
	// License & Subscription endpoints (GET)
	// NOTE: More specific routes must come BEFORE parameterized routes
	api.HandleFunc("/billing/growth-packs/available", handler.GetAvailableGrowthPacks).Methods("GET")
	api.HandleFunc("/billing/license/{tenantId}", handler.GetLicenseStatus).Methods("GET")
	api.HandleFunc("/billing/subscription/{tenantId}", handler.GetSubscription).Methods("GET")
	api.HandleFunc("/billing/growth-packs/{tenantId}", handler.GetEnabledGrowthPacks).Methods("GET")
	api.HandleFunc("/billing/usage/{tenantId}", handler.GetUsageSummary).Methods("GET")
	api.HandleFunc("/billing/validate", handler.ValidateCameraLicense).Methods("POST")
	
	// Legacy endpoints (POST) - for backwards compatibility
	api.HandleFunc("/licenses/validate", handler.ValidateLicense).Methods("POST")
	api.HandleFunc("/entitlements/check", handler.CheckEntitlement).Methods("POST")
	api.HandleFunc("/usage/batch", handler.ReportUsageBatch).Methods("POST")
	api.HandleFunc("/heartbeat", handler.Heartbeat).Methods("POST")
	
	// Admin routes
	r.HandleFunc("/health", handler.HealthCheck).Methods("GET")
	r.HandleFunc("/stats", handler.GetStats).Methods("GET")
	
	// Start server
	port := ":8081"
	log.Printf("🚀 BrinkByte Vision Billing Server starting on port %s", port)
	log.Printf("📊 New Billing API Endpoints:")
	log.Printf("   GET  http://localhost%s/api/v1/billing/license/{tenantId}", port)
	log.Printf("   GET  http://localhost%s/api/v1/billing/subscription/{tenantId}", port)
	log.Printf("   GET  http://localhost%s/api/v1/billing/growth-packs/{tenantId}", port)
	log.Printf("   GET  http://localhost%s/api/v1/billing/growth-packs/available", port)
	log.Printf("   GET  http://localhost%s/api/v1/billing/usage/{tenantId}", port)
	log.Printf("   POST http://localhost%s/api/v1/billing/validate", port)
	log.Printf("")
	log.Printf("📊 Legacy API Endpoints:")
	log.Printf("   POST http://localhost%s/api/v1/licenses/validate", port)
	log.Printf("   POST http://localhost%s/api/v1/entitlements/check", port)
	log.Printf("   POST http://localhost%s/api/v1/usage/batch", port)
	log.Printf("   POST http://localhost%s/api/v1/heartbeat", port)
	log.Printf("")
	log.Printf("📊 Admin Endpoints:")
	log.Printf("   GET  http://localhost%s/health", port)
	log.Printf("   GET  http://localhost%s/stats", port)
	log.Printf("")
	log.Printf("✨ Server ready to accept connections!")
	
	if err := http.ListenAndServe(port, r); err != nil {
		log.Fatal(err)
	}
}
