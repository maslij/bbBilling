package middleware

import (
	"log"
	"net/http"
	"time"
)

// CORS adds CORS headers to all responses
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		
		// Handle preflight
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

// Logging logs all HTTP requests
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		// Log request
		log.Printf("[HTTP] %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
		
		next.ServeHTTP(w, r)
		
		// Log response time
		duration := time.Since(start)
		log.Printf("[HTTP] %s %s completed in %v", r.Method, r.URL.Path, duration)
	})
}

