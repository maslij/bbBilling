package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"brinkbyte-billing-server/models"
	"brinkbyte-billing-server/storage"
)

type Handler struct {
	storage   *storage.InMemoryStorage
	startTime time.Time
}

func NewHandler(store *storage.InMemoryStorage) *Handler {
	return &Handler{
		storage:   store,
		startTime: time.Now(),
	}
}

// ValidateLicense handles license validation requests
func (h *Handler) ValidateLicense(w http.ResponseWriter, r *http.Request) {
	var req models.LicenseValidationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("[LICENSE] Validation request: camera=%s, tenant=%s, device=%s",
		req.CameraID, req.TenantID, req.DeviceID)

	// Check if license exists in storage
	if license, exists := h.storage.GetLicense(req.CameraID); exists {
		log.Printf("[LICENSE] Found cached license for camera=%s", req.CameraID)
		respondJSON(w, license)
		return
	}

	// Create new license (in production, this would query database)
	resp := &models.LicenseValidationResponse{
		IsValid:            true,
		LicenseMode:        "base",
		EnabledGrowthPacks: []string{"advanced_analytics", "active_transport"},
		ValidUntil:         time.Now().Add(365 * 24 * time.Hour), // 1 year
		CamerasAllowed:     100,
	}

	// Store for future requests
	h.storage.StoreLicense(req.CameraID, resp)
	
	log.Printf("[LICENSE] Created new license for camera=%s, mode=%s, packs=%v",
		req.CameraID, resp.LicenseMode, resp.EnabledGrowthPacks)

	respondJSON(w, resp)
}

// CheckEntitlement handles feature entitlement checks
func (h *Handler) CheckEntitlement(w http.ResponseWriter, r *http.Request) {
	var req models.EntitlementCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("[ENTITLEMENT] Check: tenant=%s, category=%s, feature=%s",
		req.TenantID, req.FeatureCategory, req.FeatureName)

	// In production, this would check against database
	resp := models.EntitlementCheckResponse{
		IsEnabled:      true,
		QuotaRemaining: 50000, // API calls remaining
		ValidUntil:     time.Now().Add(30 * 24 * time.Hour),
	}

	log.Printf("[ENTITLEMENT] Feature %s/%s enabled for tenant %s",
		req.FeatureCategory, req.FeatureName, req.TenantID)

	respondJSON(w, resp)
}

// ReportUsageBatch handles batch usage reporting
func (h *Handler) ReportUsageBatch(w http.ResponseWriter, r *http.Request) {
	var req models.UsageBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("[USAGE] Batch received: %d events", len(req.Events))

	// Store events
	h.storage.AddUsageEvents(req.Events)

	// Log summary
	for _, event := range req.Events {
		log.Printf("[USAGE]   - %s: %s = %.2f %s (tenant=%s)",
			event.EventType, event.ResourceID, event.Quantity, event.Unit, event.TenantID)
	}

	resp := models.UsageBatchResponse{
		AcceptedCount: len(req.Events),
		RejectedCount: 0,
		Errors:        []string{},
	}

	respondJSON(w, resp)
}

// Heartbeat handles device heartbeat requests
func (h *Handler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	var req models.HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("[HEARTBEAT] Device: %s, tenant=%s, cameras=%d, tier=%s",
		req.DeviceID, req.TenantID, len(req.ActiveCameraIDs), req.ManagementTier)

	resp := models.HeartbeatResponse{
		Status:               "ok",
		NextHeartbeatSeconds: 900, // 15 minutes
	}

	respondJSON(w, resp)
}

// GetLicenseStatus returns the license status for a tenant
func (h *Handler) GetLicenseStatus(w http.ResponseWriter, r *http.Request) {
	// Extract tenant ID from URL path (expecting /api/v1/billing/license/{tenantId})
	tenantID := r.URL.Path[len("/api/v1/billing/license/"):]
	
	log.Printf("[LICENSE_STATUS] Request for tenant: %s", tenantID)
	
	// For development, return mock license status with trial time calculation
	trialStartDate := time.Now().AddDate(0, -1, 0)  // Started 1 month ago
	trialEndDate := time.Now().AddDate(0, 2, 0)    // Ends in 2 months (3 month trial)
	
	// Calculate days remaining
	daysRemaining := int(time.Until(trialEndDate).Hours() / 24)
	
	// Determine license mode based on trial status
	licenseMode := "trial"
	var daysRemainingPtr *int = &daysRemaining
	trialMaxCameras := 2
	
	// If trial expired, switch to unlicensed mode
	if daysRemaining < 0 {
		licenseMode = "unlicensed"
		daysRemainingPtr = nil
		trialMaxCameras = 0
	}
	
	resp := map[string]interface{}{
		"license_mode":         licenseMode,
		"is_valid":             daysRemaining >= 0,
		"active_cameras":       2,
		"cameras_allowed":      nil, // nil for unlimited (base license), number for trial
		"trial_max_cameras":    trialMaxCameras,
		"days_remaining":       daysRemainingPtr,
		"valid_until":          trialEndDate.Format(time.RFC3339),
		"trial_started_at":     trialStartDate.Format(time.RFC3339),
		"enabled_growth_packs": []string{"Advanced Analytics", "Active Transport"},
		"cameras": []map[string]interface{}{
			{
				"camera_id":            "camera-1",
				"tenant_id":            tenantID,
				"mode":                 licenseMode,
				"start_date":           trialStartDate.Format(time.RFC3339),
				"end_date":             trialEndDate.Format(time.RFC3339),
				"days_remaining":       daysRemainingPtr,
				"is_expired":           daysRemaining < 0,
				"enabled_growth_packs": []string{"Advanced Analytics", "Active Transport"},
			},
		},
	}
	
	respondJSON(w, resp)
}

// GetSubscription returns subscription information for a tenant
func (h *Handler) GetSubscription(w http.ResponseWriter, r *http.Request) {
	// Extract tenant ID from URL path
	tenantID := r.URL.Path[len("/api/v1/billing/subscription/"):]
	
	log.Printf("[SUBSCRIPTION] Request for tenant: %s", tenantID)
	
	resp := map[string]interface{}{
		"subscription_id":    "sub-123",
		"tenant_id":          tenantID,
		"plan":               "base",
		"status":             "active",
		"cameras_licensed":   10,
		"growth_packs": []map[string]interface{}{
			{
				"pack_id":       "pack-advanced-analytics",
				"pack_name":     "Advanced Analytics",
				"enabled_at":    time.Now().AddDate(0, -1, 0).Format(time.RFC3339),
				"price_monthly": 50.00,
			},
			{
				"pack_id":       "pack-active-transport",
				"pack_name":     "Active Transport",
				"enabled_at":    time.Now().AddDate(0, -1, 0).Format(time.RFC3339),
				"price_monthly": 30.00,
			},
		},
		"billing_cycle":      "monthly",
		"next_billing_date":  time.Now().AddDate(0, 1, 0).Format(time.RFC3339),
		"total_monthly_cost": 680.00, // 10 cameras * $60 + $50 + $30
	}
	
	respondJSON(w, resp)
}

// GetEnabledGrowthPacks returns enabled growth packs for a tenant
func (h *Handler) GetEnabledGrowthPacks(w http.ResponseWriter, r *http.Request) {
	// Extract tenant ID from URL path
	tenantID := r.URL.Path[len("/api/v1/billing/growth-packs/"):]
	
	log.Printf("[GROWTH_PACKS] Request for tenant: %s", tenantID)
	
	resp := map[string]interface{}{
		"enabled_packs": []string{
			"Advanced Analytics",
			"Active Transport",
		},
	}
	
	respondJSON(w, resp)
}

// GetAvailableGrowthPacks returns all available growth packs
func (h *Handler) GetAvailableGrowthPacks(w http.ResponseWriter, r *http.Request) {
	log.Printf("[GROWTH_PACKS] Available packs request")
	
	resp := map[string]interface{}{
		"packs": []map[string]interface{}{
			{
				"pack_id":       "pack-advanced-analytics",
				"pack_name":     "Advanced Analytics",
				"description":   "Advanced analytics and reporting features",
				"category":      "analytics",
				"price_monthly": 50.00,
				"features": []string{
					"Real-time analytics dashboard",
					"Historical data analysis",
					"Custom reports",
					"Advanced visualizations",
				},
				"is_enabled": true,
			},
			{
				"pack_id":       "pack-active-transport",
				"pack_name":     "Active Transport",
				"description":   "Active transport mode detection and analytics",
				"category":      "intelligence",
				"price_monthly": 30.00,
				"features": []string{
					"Pedestrian detection",
					"Cyclist detection",
					"E-scooter detection",
					"Movement pattern analysis",
				},
				"is_enabled": true,
			},
			{
				"pack_id":       "pack-cloud-storage",
				"pack_name":     "Cloud Storage",
				"description":   "Extended cloud storage for video and analytics data",
				"category":      "data",
				"price_monthly": 100.00,
				"features": []string{
					"1TB cloud storage",
					"30-day retention",
					"Encrypted backups",
					"High-availability storage",
				},
				"is_enabled": false,
			},
			{
				"pack_id":       "pack-api-integration",
				"pack_name":     "API Integration",
				"description":   "Advanced API access and integration capabilities",
				"category":      "integration",
				"price_monthly": 75.00,
				"features": []string{
					"Unlimited API calls",
					"Webhook support",
					"Custom integrations",
					"Priority support",
				},
				"is_enabled": false,
			},
		},
	}
	
	respondJSON(w, resp)
}

// GetUsageSummary returns usage summary for a tenant
func (h *Handler) GetUsageSummary(w http.ResponseWriter, r *http.Request) {
	// Extract tenant ID from URL path
	tenantID := r.URL.Path[len("/api/v1/billing/usage/"):]
	
	log.Printf("[USAGE_SUMMARY] Request for tenant: %s", tenantID)
	
	// Parse query parameters for date range
	query := r.URL.Query()
	periodStart := query.Get("start")
	periodEnd := query.Get("end")
	
	if periodStart == "" {
		periodStart = time.Now().AddDate(0, -1, 0).Format(time.RFC3339)
	}
	if periodEnd == "" {
		periodEnd = time.Now().Format(time.RFC3339)
	}
	
	resp := map[string]interface{}{
		"tenant_id":          tenantID,
		"period_start":       periodStart,
		"period_end":         periodEnd,
		"api_calls":          12450,
		"llm_tokens_used":    45000,
		"storage_gb_days":    150.5,
		"sms_sent":           25,
		"agent_executions":   340,
	}
	
	respondJSON(w, resp)
}

// ValidateCameraLicense validates a specific camera license
func (h *Handler) ValidateCameraLicense(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CameraID string `json:"camera_id"`
		TenantID string `json:"tenant_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("[CAMERA_VALIDATE] camera=%s, tenant=%s", req.CameraID, req.TenantID)

	resp := map[string]interface{}{
		"is_valid":             true,
		"license_mode":         "base",
		"valid_until":          time.Now().Add(365 * 24 * time.Hour).Format(time.RFC3339),
		"enabled_growth_packs": []string{"Advanced Analytics", "Active Transport"},
	}

	respondJSON(w, resp)
}

// HealthCheck returns server health status
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	resp := models.HealthResponse{
		Status:        "healthy",
		Service:       "brinkbyte-vision-billing",
		Version:       "1.0.0",
		Timestamp:     time.Now(),
		UptimeSeconds: time.Since(h.startTime).Seconds(),
		TotalEvents:   h.storage.GetUsageEventCount(),
	}

	respondJSON(w, resp)
}

// GetStats returns usage statistics
func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	resp := models.StatsResponse{
		TotalEvents: h.storage.GetUsageEventCount(),
		ByType:      h.storage.GetUsageStats(),
		Tenants:     h.storage.GetTenantCount(),
	}

	respondJSON(w, resp)
}

// Helper function to send JSON responses
func respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("[ERROR] Failed to encode JSON response: %v", err)
	}
}

