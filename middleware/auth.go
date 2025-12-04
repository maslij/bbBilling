package middleware

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"brinkbyte-billing-server/models"
)

// ContextKey is a custom type for context keys to avoid collisions
type ContextKey string

const (
	// TenantContextKey is the context key for tenant information
	TenantContextKey ContextKey = "tenant"
)

// Storage interface for auth middleware (minimal subset)
type Storage interface {
	GetTenantByAPIKey(ctx context.Context, apiKey string) (*models.Tenant, error)
}

// AuthMiddleware validates API key and attaches tenant info to context
func AuthMiddleware(store Storage) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth for OPTIONS requests (CORS preflight)
			if r.Method == "OPTIONS" {
				next.ServeHTTP(w, r)
				return
			}

			// Get Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				respondError(w, http.StatusUnauthorized, "Missing Authorization header")
				return
			}

			// Parse Bearer token
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				respondError(w, http.StatusUnauthorized, "Invalid Authorization header format")
				return
			}

			apiKey := parts[1]
			if apiKey == "" {
				respondError(w, http.StatusUnauthorized, "Empty API key")
				return
			}

			// Look up tenant by API key
			tenant, err := store.GetTenantByAPIKey(r.Context(), apiKey)
			if err != nil {
				log.Printf("[AUTH] Error looking up API key: %v", err)
				respondError(w, http.StatusInternalServerError, "Authentication error")
				return
			}

			if tenant == nil {
				log.Printf("[AUTH] Invalid API key: %s...", apiKey[:min(10, len(apiKey))])
				respondError(w, http.StatusUnauthorized, "Invalid API key")
				return
			}

			// Check tenant status
			if tenant.Status != "active" {
				log.Printf("[AUTH] Tenant %s is not active (status: %s)", tenant.ID, tenant.Status)
				respondError(w, http.StatusForbidden, "Tenant account is not active")
				return
			}

			// Attach tenant to context
			ctx := context.WithValue(r.Context(), TenantContextKey, tenant)
			log.Printf("[AUTH] Authenticated tenant: %s (%s)", tenant.ID, tenant.Name)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AdminAuthMiddleware validates admin API key for admin endpoints
func AdminAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for OPTIONS requests (CORS preflight)
		if r.Method == "OPTIONS" {
			next.ServeHTTP(w, r)
			return
		}

		// Get admin API key from environment
		adminKey := os.Getenv("ADMIN_API_KEY")
		if adminKey == "" {
			// If no admin key configured, allow access (development mode)
			log.Printf("[ADMIN_AUTH] Warning: ADMIN_API_KEY not set, allowing unauthenticated access")
			next.ServeHTTP(w, r)
			return
		}

		// Get Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			respondError(w, http.StatusUnauthorized, "Missing Authorization header")
			return
		}

		// Parse Bearer token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			respondError(w, http.StatusUnauthorized, "Invalid Authorization header format")
			return
		}

		apiKey := parts[1]
		if apiKey != adminKey {
			log.Printf("[ADMIN_AUTH] Invalid admin API key attempt")
			respondError(w, http.StatusUnauthorized, "Invalid admin API key")
			return
		}

		log.Printf("[ADMIN_AUTH] Admin access granted for %s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

// GetTenantFromContext retrieves the tenant from request context
func GetTenantFromContext(r *http.Request) *models.Tenant {
	if tenant, ok := r.Context().Value(TenantContextKey).(*models.Tenant); ok {
		return tenant
	}
	return nil
}

// OptionalAuthMiddleware attaches tenant info if API key provided, but doesn't require it
func OptionalAuthMiddleware(store Storage) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				// No auth header, continue without tenant context
				next.ServeHTTP(w, r)
				return
			}

			// Parse Bearer token
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				// Invalid format, continue without tenant context
				next.ServeHTTP(w, r)
				return
			}

			apiKey := parts[1]
			if apiKey == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Look up tenant by API key
			tenant, err := store.GetTenantByAPIKey(r.Context(), apiKey)
			if err != nil || tenant == nil || tenant.Status != "active" {
				// Invalid or inactive, continue without tenant context
				next.ServeHTTP(w, r)
				return
			}

			// Attach tenant to context
			ctx := context.WithValue(r.Context(), TenantContextKey, tenant)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// helper function
func respondError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

