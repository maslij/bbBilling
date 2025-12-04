package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"brinkbyte-billing-server/models"
)

// Storage interface for all storage implementations
type Storage interface {
	// Tenant operations
	GetTenant(ctx context.Context, tenantID string) (*models.Tenant, error)
	GetTenantByAPIKey(ctx context.Context, apiKey string) (*models.Tenant, error)
	CreateTenant(ctx context.Context, tenant *models.Tenant) error
	UpdateTenant(ctx context.Context, tenant *models.Tenant) error

	// Subscription operations
	GetSubscription(ctx context.Context, tenantID string) (*models.Subscription, error)
	CreateSubscription(ctx context.Context, sub *models.Subscription) error
	UpdateSubscription(ctx context.Context, sub *models.Subscription) error

	// Growth pack operations
	GetEnabledGrowthPacks(ctx context.Context, tenantID string) ([]models.GrowthPackAssignment, error)
	EnableGrowthPack(ctx context.Context, assignment *models.GrowthPackAssignment) error
	DisableGrowthPack(ctx context.Context, tenantID, packName string) error

	// Camera license operations
	GetCameraLicense(ctx context.Context, cameraID, tenantID string) (*models.CameraLicense, error)
	SaveCameraLicense(ctx context.Context, license *models.CameraLicense) error
	GetCamerasByTenant(ctx context.Context, tenantID string) ([]models.CameraLicense, error)
	CountCamerasByTenant(ctx context.Context, tenantID string) (int, error)

	// Entitlement operations
	GetEntitlement(ctx context.Context, tenantID, category, feature string) (*models.FeatureEntitlement, error)
	SaveEntitlement(ctx context.Context, ent *models.FeatureEntitlement) error

	// Usage operations
	SaveUsageEvents(ctx context.Context, events []models.UsageEvent) error
	GetUsageSummary(ctx context.Context, tenantID string, start, end time.Time) (map[string]float64, error)

	// Edge device operations
	SaveEdgeDevice(ctx context.Context, device *models.EdgeDevice) error
	GetEdgeDevice(ctx context.Context, deviceID string) (*models.EdgeDevice, error)

	// Statistics
	GetStats(ctx context.Context) (map[string]int, error)
}

type Handler struct {
	storage   Storage
	startTime time.Time
}

func NewHandler(store Storage) *Handler {
	return &Handler{
		storage:   store,
		startTime: time.Now(),
	}
}

// ValidateLicense handles license validation requests (Legacy C++ endpoint)
func (h *Handler) ValidateLicense(w http.ResponseWriter, r *http.Request) {
	var req models.LicenseValidationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	log.Printf("[LICENSE] Validation request: camera=%s, tenant=%s, device=%s",
		req.CameraID, req.TenantID, req.DeviceID)

	// Get tenant's subscription
	sub, err := h.storage.GetSubscription(ctx, req.TenantID)
	if err != nil {
		log.Printf("[LICENSE] Error getting subscription: %v", err)
		respondError(w, http.StatusInternalServerError, "Failed to get subscription")
		return
	}

	// Get enabled growth packs
	packs, err := h.storage.GetEnabledGrowthPacks(ctx, req.TenantID)
	if err != nil {
		log.Printf("[LICENSE] Error getting growth packs: %v", err)
	}

	var enabledPackNames []string
	for _, pack := range packs {
		enabledPackNames = append(enabledPackNames, pack.PackName)
	}

	// Determine license status
	var resp *models.LicenseValidationResponse

	if sub == nil {
		// No subscription - create a trial for new tenants
		log.Printf("[LICENSE] No subscription found for tenant %s, creating trial", req.TenantID)

		// Create tenant if doesn't exist
		tenant, _ := h.storage.GetTenant(ctx, req.TenantID)
		if tenant == nil {
			now := time.Now()
			tenant = &models.Tenant{
				ID:        req.TenantID,
				Name:      "Auto-created Tenant",
				Status:    "active",
				CreatedAt: now,
				UpdatedAt: now,
			}
			h.storage.CreateTenant(ctx, tenant)
		}

		// Create trial subscription
		now := time.Now()
		trialEnd := now.AddDate(0, 3, 0) // 3 month trial
		sub = &models.Subscription{
			ID:             uuid.New().String(),
			TenantID:       req.TenantID,
			Plan:           "trial",
			Status:         "active",
			CamerasLicensed: 2,
			TrialStartDate: &now,
			TrialEndDate:   &trialEnd,
			BillingCycle:   "monthly",
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		h.storage.CreateSubscription(ctx, sub)

		resp = &models.LicenseValidationResponse{
			IsValid:            true,
			LicenseMode:        "trial",
			EnabledGrowthPacks: enabledPackNames,
			ValidUntil:         trialEnd,
			CamerasAllowed:     2,
		}
	} else {
		// Check subscription status
		isValid := sub.Status == "active"
		licenseMode := sub.Plan

		// Check camera count for trial
		camerasAllowed := sub.CamerasLicensed
		if sub.Plan == "trial" {
			cameraCount, _ := h.storage.CountCamerasByTenant(ctx, req.TenantID)
			if cameraCount >= sub.CamerasLicensed && req.CameraID != "" {
				// Check if this camera is already registered
				existingLicense, _ := h.storage.GetCameraLicense(ctx, req.CameraID, req.TenantID)
				if existingLicense == nil {
					isValid = false
					log.Printf("[LICENSE] Trial camera limit exceeded for tenant %s", req.TenantID)
				}
			}
		}

		// Check expiry
		var validUntil time.Time
		if sub.Plan == "trial" && sub.TrialEndDate != nil {
			validUntil = *sub.TrialEndDate
			if time.Now().After(validUntil) {
				isValid = false
				licenseMode = "expired"
			}
		} else if sub.SubscriptionEndDate != nil {
			validUntil = *sub.SubscriptionEndDate
			if time.Now().After(validUntil) {
				isValid = false
				licenseMode = "expired"
			}
		} else {
			// No expiry set - default to 1 year
			validUntil = time.Now().AddDate(1, 0, 0)
		}

		resp = &models.LicenseValidationResponse{
			IsValid:            isValid,
			LicenseMode:        licenseMode,
			EnabledGrowthPacks: enabledPackNames,
			ValidUntil:         validUntil,
			CamerasAllowed:     camerasAllowed,
		}
	}

	// Save/update camera license
	if req.CameraID != "" {
		packsJSON, _ := json.Marshal(resp.EnabledGrowthPacks)
		license := &models.CameraLicense{
			ID:                 uuid.New().String(),
			CameraID:           req.CameraID,
			TenantID:           req.TenantID,
			DeviceID:           &req.DeviceID,
			LicenseMode:        resp.LicenseMode,
			IsValid:            resp.IsValid,
			ValidUntil:         &resp.ValidUntil,
			EnabledGrowthPacks: packsJSON,
			LastValidated:      time.Now(),
			CreatedAt:          time.Now(),
			UpdatedAt:          time.Now(),
		}
		h.storage.SaveCameraLicense(ctx, license)
	}

	log.Printf("[LICENSE] Response for camera=%s: valid=%v, mode=%s, packs=%v",
		req.CameraID, resp.IsValid, resp.LicenseMode, resp.EnabledGrowthPacks)

	respondJSON(w, resp)
}

// CheckEntitlement handles feature entitlement checks
func (h *Handler) CheckEntitlement(w http.ResponseWriter, r *http.Request) {
	var req models.EntitlementCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	log.Printf("[ENTITLEMENT] Check: tenant=%s, category=%s, feature=%s",
		req.TenantID, req.FeatureCategory, req.FeatureName)

	// Check base features first
	baseFeatures := models.BaseFeatures()
	if features, ok := baseFeatures[req.FeatureCategory]; ok {
		for _, f := range features {
			if f == req.FeatureName {
				// Feature is in base license - always enabled
				resp := models.EntitlementCheckResponse{
					IsEnabled:      true,
					QuotaRemaining: -1, // Unlimited
					ValidUntil:     time.Now().AddDate(1, 0, 0),
				}
				log.Printf("[ENTITLEMENT] Feature %s/%s is base feature, enabled",
					req.FeatureCategory, req.FeatureName)
				respondJSON(w, resp)
				return
			}
		}
	}

	// Check growth pack features
	packs, _ := h.storage.GetEnabledGrowthPacks(ctx, req.TenantID)
	for _, pack := range packs {
		packFeatures := models.GetGrowthPackFeatures(pack.PackName)
		if features, ok := packFeatures[req.FeatureCategory]; ok {
			for _, f := range features {
				if f == req.FeatureName {
					// Feature is enabled via growth pack
					resp := models.EntitlementCheckResponse{
						IsEnabled:      true,
						QuotaRemaining: -1,
						ValidUntil:     time.Now().AddDate(1, 0, 0),
					}
					log.Printf("[ENTITLEMENT] Feature %s/%s enabled via pack %s",
						req.FeatureCategory, req.FeatureName, pack.PackName)
					respondJSON(w, resp)
					return
				}
			}
		}
	}

	// Check stored entitlement
	ent, _ := h.storage.GetEntitlement(ctx, req.TenantID, req.FeatureCategory, req.FeatureName)
	if ent != nil && ent.IsEnabled {
		quotaRemaining := -1
		if ent.QuotaLimit > 0 {
			quotaRemaining = ent.QuotaLimit - ent.QuotaUsed
			if quotaRemaining < 0 {
				quotaRemaining = 0
			}
		}

		validUntil := time.Now().AddDate(1, 0, 0)
		if ent.ValidUntil != nil {
			validUntil = *ent.ValidUntil
		}

		resp := models.EntitlementCheckResponse{
			IsEnabled:      true,
			QuotaRemaining: quotaRemaining,
			ValidUntil:     validUntil,
		}
		respondJSON(w, resp)
		return
	}

	// Feature not enabled
	resp := models.EntitlementCheckResponse{
		IsEnabled:      false,
		QuotaRemaining: 0,
		ValidUntil:     time.Now(),
	}
	log.Printf("[ENTITLEMENT] Feature %s/%s not enabled for tenant %s",
		req.FeatureCategory, req.FeatureName, req.TenantID)
	respondJSON(w, resp)
}

// ReportUsageBatch handles batch usage reporting
func (h *Handler) ReportUsageBatch(w http.ResponseWriter, r *http.Request) {
	var req models.UsageBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[USAGE] Error decoding request: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	log.Printf("[USAGE] Batch received: %d events", len(req.Events))

	// Convert to storage format and save
	var usageEvents []models.UsageEvent
	for _, event := range req.Events {
		// Handle event_time - FlexibleTime handles Unix timestamp strings
		eventTime := event.EventTime.Time
		if eventTime.IsZero() {
			eventTime = time.Now()
		}

		metadataJSON, _ := json.Marshal(event.Metadata)
		usageEvents = append(usageEvents, models.UsageEvent{
			TenantID:   event.TenantID,
			EventType:  event.EventType,
			ResourceID: event.ResourceID,
			Quantity:   event.Quantity,
			Unit:       event.Unit,
			Metadata:   metadataJSON,
			EventTime:  eventTime,
		})

		log.Printf("[USAGE]   - %s: %s = %.2f %s (tenant=%s, time=%v)",
			event.EventType, event.ResourceID, event.Quantity, event.Unit, event.TenantID, eventTime)
	}

	err := h.storage.SaveUsageEvents(ctx, usageEvents)
	if err != nil {
		log.Printf("[USAGE] Error saving events: %v", err)
		respondJSON(w, models.UsageBatchResponse{
			AcceptedCount: 0,
			RejectedCount: len(req.Events),
			Errors:        []string{err.Error()},
		})
		return
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

	ctx := r.Context()
	log.Printf("[HEARTBEAT] Device: %s, tenant=%s, cameras=%d, tier=%s",
		req.DeviceID, req.TenantID, len(req.ActiveCameraIDs), req.ManagementTier)

	// Save/update edge device
	now := time.Now()
	device := &models.EdgeDevice{
		ID:                uuid.New().String(),
		DeviceID:          req.DeviceID,
		TenantID:          req.TenantID,
		Status:            "active",
		ManagementTier:    req.ManagementTier,
		LastHeartbeat:     &now,
		ActiveCameraCount: len(req.ActiveCameraIDs),
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	h.storage.SaveEdgeDevice(ctx, device)

	resp := models.HeartbeatResponse{
		Status:               "ok",
		NextHeartbeatSeconds: 900, // 15 minutes
	}

	respondJSON(w, resp)
}

// GetLicenseStatus returns the license status for a tenant
func (h *Handler) GetLicenseStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tenantID := vars["tenantId"]

	ctx := r.Context()
	log.Printf("[LICENSE_STATUS] Request for tenant: %s", tenantID)

	// Get subscription
	sub, err := h.storage.GetSubscription(ctx, tenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to get subscription")
		return
	}

	// Get cameras
	cameras, _ := h.storage.GetCamerasByTenant(ctx, tenantID)

	// Get growth packs
	packs, _ := h.storage.GetEnabledGrowthPacks(ctx, tenantID)
	var enabledPackNames []string
	for _, pack := range packs {
		enabledPackNames = append(enabledPackNames, pack.PackName)
	}

	if sub == nil {
		// Auto-create trial subscription for new tenants
		log.Printf("[LICENSE_STATUS] No subscription found for tenant %s, auto-creating trial", tenantID)
		
		// First ensure tenant exists
		tenant, _ := h.storage.GetTenant(ctx, tenantID)
		if tenant == nil {
			// Create tenant
			newTenant := &models.Tenant{
				ID:     tenantID,
				Name:   "Auto-created Tenant",
				Email:  nil,
				Status: "active",
			}
			if err := h.storage.CreateTenant(ctx, newTenant); err != nil {
				log.Printf("[LICENSE_STATUS] Failed to create tenant: %v", err)
				// Continue anyway - subscription might still work
			}
		}
		
		// Create trial subscription
		trialStart := time.Now()
		trialEnd := trialStart.AddDate(0, 0, models.TrialDurationDays)
		newSub := &models.Subscription{
			ID:              uuid.New().String(),
			TenantID:        tenantID,
			Plan:            "trial",
			Status:          "active",
			CamerasLicensed: models.TrialMaxCameras,
			TrialStartDate:  &trialStart,
			TrialEndDate:    &trialEnd,
			BillingCycle:    "monthly",
		}
		
		if err := h.storage.CreateSubscription(ctx, newSub); err != nil {
			log.Printf("[LICENSE_STATUS] Failed to create trial subscription: %v", err)
			// Return unlicensed status if we can't create subscription
			resp := map[string]interface{}{
				"license_mode":         "unlicensed",
				"is_valid":             false,
				"active_cameras":       0,
				"cameras_allowed":      0,
				"enabled_growth_packs": []string{},
				"cameras":              []interface{}{},
			}
			respondJSON(w, resp)
			return
		}
		
		// Use the newly created subscription
		sub = newSub
		log.Printf("[LICENSE_STATUS] Auto-created trial subscription for tenant %s", tenantID)
	}

	// Build response
	var daysRemaining *int
	var validUntil string
	var trialStartedAt string
	licenseMode := sub.Plan

	if sub.Plan == "trial" && sub.TrialEndDate != nil {
		days := int(time.Until(*sub.TrialEndDate).Hours() / 24)
		daysRemaining = &days
		validUntil = sub.TrialEndDate.Format(time.RFC3339)
		if sub.TrialStartDate != nil {
			trialStartedAt = sub.TrialStartDate.Format(time.RFC3339)
		}
		if days < 0 {
			licenseMode = "expired"
		}
	} else if sub.SubscriptionEndDate != nil {
		validUntil = sub.SubscriptionEndDate.Format(time.RFC3339)
	} else {
		validUntil = time.Now().AddDate(1, 0, 0).Format(time.RFC3339)
	}

	// Convert cameras to response format
	var cameraList []map[string]interface{}
	for _, cam := range cameras {
		var packs []string
		json.Unmarshal(cam.EnabledGrowthPacks, &packs)

		camResp := map[string]interface{}{
			"camera_id":            cam.CameraID,
			"tenant_id":            cam.TenantID,
			"mode":                 cam.LicenseMode,
			"is_valid":             cam.IsValid,
			"enabled_growth_packs": packs,
			"created_at":           cam.CreatedAt.Format(time.RFC3339),
		}
		if cam.ValidUntil != nil {
			camResp["valid_until"] = cam.ValidUntil.Format(time.RFC3339)
		}
		cameraList = append(cameraList, camResp)
	}

	// Calculate pricing
	pricing := calculatePricing(len(cameras), packs)

	// Mask the license key (show only last 4 characters)
	maskedKey := maskLicenseKey(tenantID)

	resp := map[string]interface{}{
		"license_mode":         licenseMode,
		"is_valid":             sub.Status == "active" && (daysRemaining == nil || *daysRemaining >= 0),
		"active_cameras":       len(cameras),
		"cameras_allowed":      sub.CamerasLicensed,
		"days_remaining":       daysRemaining,
		"valid_until":          validUntil,
		"enabled_growth_packs": enabledPackNames,
		"cameras":              cameraList,
		"pricing":              pricing,
		"license_key":          maskedKey,
		"can_revoke":           licenseMode == "base", // Can only revoke base licenses
		"trial_max_cameras":    models.TrialMaxCameras, // Always return trial limit for UI
	}

	if trialStartedAt != "" {
		resp["trial_started_at"] = trialStartedAt
	}

	respondJSON(w, resp)
}

// maskLicenseKey masks all but the last 4 characters of a license key
func maskLicenseKey(key string) string {
	if len(key) <= 4 {
		return key
	}
	masked := strings.Repeat("*", len(key)-4) + key[len(key)-4:]
	return masked
}

// Base pricing configuration (AUD)
const (
	BasePerCameraRate = 14.99   // Base license per camera per month (AUD)
	DefaultCurrency   = "AUD"
)

// PricingBreakdown represents the cost breakdown
type PricingBreakdown struct {
	BaseCost       float64            `json:"base_cost"`
	CameraCount    int                `json:"camera_count"`
	PerCameraRate  float64            `json:"per_camera_rate"`
	GrowthPacks    map[string]float64 `json:"growth_packs"`
	GrowthPackCost float64            `json:"growth_pack_cost"`
	TotalMonthly   float64            `json:"total_monthly"`
	Currency       string             `json:"currency"`
}

// getGrowthPackPrice looks up the price for a growth pack from the model
func getGrowthPackPrice(packName string) float64 {
	for _, pack := range models.AvailableGrowthPacks() {
		if pack.PackName == packName {
			return pack.PriceMonthly
		}
	}
	return 0
}

func calculatePricing(cameraCount int, packs []models.GrowthPackAssignment) PricingBreakdown {
	baseCost := float64(cameraCount) * BasePerCameraRate

	growthPackCosts := make(map[string]float64)
	var growthPackTotal float64

	for _, pack := range packs {
		// First check if pack has custom price, otherwise use model price
		if pack.PriceMonthly != nil && *pack.PriceMonthly > 0 {
			growthPackCosts[pack.PackName] = *pack.PriceMonthly
			growthPackTotal += *pack.PriceMonthly
		} else {
			price := getGrowthPackPrice(pack.PackName)
			growthPackCosts[pack.PackName] = price
			growthPackTotal += price
		}
	}

	totalMonthly := baseCost + growthPackTotal

	return PricingBreakdown{
		BaseCost:       baseCost,
		CameraCount:    cameraCount,
		PerCameraRate:  BasePerCameraRate,
		GrowthPacks:    growthPackCosts,
		GrowthPackCost: growthPackTotal,
		TotalMonthly:   totalMonthly,
		Currency:       DefaultCurrency,
	}
}

// GetSubscription returns subscription information for a tenant
func (h *Handler) GetSubscription(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tenantID := vars["tenantId"]

	ctx := r.Context()
	log.Printf("[SUBSCRIPTION] Request for tenant: %s", tenantID)

	sub, err := h.storage.GetSubscription(ctx, tenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to get subscription")
		return
	}

	if sub == nil {
		respondError(w, http.StatusNotFound, "Subscription not found")
		return
	}

	// Get growth packs
	packs, _ := h.storage.GetEnabledGrowthPacks(ctx, tenantID)
	var growthPackDetails []map[string]interface{}
	var totalMonthly float64

	for _, pack := range packs {
		packInfo := map[string]interface{}{
			"pack_name":  pack.PackName,
			"enabled_at": pack.EnabledAt.Format(time.RFC3339),
		}
		if pack.PriceMonthly != nil {
			packInfo["price_monthly"] = *pack.PriceMonthly
			totalMonthly += *pack.PriceMonthly
		}
		growthPackDetails = append(growthPackDetails, packInfo)
	}

	// Calculate base cost
	cameraCost := float64(sub.CamerasLicensed) * 60.0 // $60/camera/month
	totalMonthly += cameraCost

	var nextBillingDate string
	if sub.SubscriptionEndDate != nil {
		nextBillingDate = sub.SubscriptionEndDate.Format(time.RFC3339)
	} else if sub.Plan != "trial" {
		nextBillingDate = time.Now().AddDate(0, 1, 0).Format(time.RFC3339)
	}

	resp := map[string]interface{}{
		"subscription_id":    sub.ID,
		"tenant_id":          sub.TenantID,
		"plan":               sub.Plan,
		"status":             sub.Status,
		"cameras_licensed":   sub.CamerasLicensed,
		"growth_packs":       growthPackDetails,
		"billing_cycle":      sub.BillingCycle,
		"next_billing_date":  nextBillingDate,
		"total_monthly_cost": totalMonthly,
	}

	respondJSON(w, resp)
}

// GetEnabledGrowthPacks returns enabled growth packs for a tenant
func (h *Handler) GetEnabledGrowthPacks(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tenantID := vars["tenantId"]

	ctx := r.Context()
	log.Printf("[GROWTH_PACKS] Request for tenant: %s", tenantID)

	packs, err := h.storage.GetEnabledGrowthPacks(ctx, tenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to get growth packs")
		return
	}

	var enabledPacks []string
	for _, pack := range packs {
		enabledPacks = append(enabledPacks, pack.PackName)
	}

	resp := map[string]interface{}{
		"enabled_packs": enabledPacks,
	}

	respondJSON(w, resp)
}

// GetAvailableGrowthPacks returns all available growth packs
func (h *Handler) GetAvailableGrowthPacks(w http.ResponseWriter, r *http.Request) {
	log.Printf("[GROWTH_PACKS] Available packs request")

	packs := models.AvailableGrowthPacks()

	var packList []map[string]interface{}
	for _, pack := range packs {
		packList = append(packList, map[string]interface{}{
			"pack_id":       pack.PackID,
			"pack_name":     pack.PackName,
			"description":   pack.Description,
			"category":      pack.Category,
			"price_monthly": pack.PriceMonthly,
			"features":      pack.Features,
		})
	}

	resp := map[string]interface{}{
		"packs": packList,
	}

	respondJSON(w, resp)
}

// GetPricingConfig returns the pricing configuration
func (h *Handler) GetPricingConfig(w http.ResponseWriter, r *http.Request) {
	log.Printf("[PRICING] Config request")

	// Get all available growth packs with their prices
	packs := models.AvailableGrowthPacks()

	var growthPackPricing []map[string]interface{}
	for _, pack := range packs {
		growthPackPricing = append(growthPackPricing, map[string]interface{}{
			"pack_id":       pack.PackID,
			"pack_name":     pack.PackName,
			"category":      pack.Category,
			"price_monthly": pack.PriceMonthly,
			"description":   pack.Description,
		})
	}

	resp := map[string]interface{}{
		"base_license": map[string]interface{}{
			"per_camera_monthly": BasePerCameraRate,
			"description":        "Base license per camera per month",
		},
		"growth_packs": growthPackPricing,
		"currency":     DefaultCurrency,
	}

	respondJSON(w, resp)
}

// RevokeLicense handles license revocation - switches back to trial mode
// preserving the original trial time (does NOT reset trial period)
func (h *Handler) RevokeLicense(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tenantID := vars["tenantId"]

	ctx := r.Context()
	log.Printf("[REVOKE] License revocation request for tenant: %s", tenantID)

	// Get current subscription
	sub, err := h.storage.GetSubscription(ctx, tenantID)
	if err != nil || sub == nil {
		respondError(w, http.StatusNotFound, "Subscription not found")
		return
	}

	// Can only revoke a base license (not trial)
	if sub.Plan == "trial" {
		respondError(w, http.StatusBadRequest, "Cannot revoke a trial license")
		return
	}

	// Get current camera count BEFORE revoking
	cameras, _ := h.storage.GetCamerasByTenant(ctx, tenantID)
	currentCameraCount := len(cameras)
	camerasToStop := 0
	if currentCameraCount > models.TrialMaxCameras {
		camerasToStop = currentCameraCount - models.TrialMaxCameras
	}

	log.Printf("[REVOKE] Current cameras: %d, Trial limit: %d, Cameras to stop: %d",
		currentCameraCount, models.TrialMaxCameras, camerasToStop)

	// Revert to trial mode - preserve original trial dates
	sub.Plan = "trial"
	sub.Status = "active"
	sub.CamerasLicensed = models.TrialMaxCameras // Use constant, not hardcoded

	// If trial dates are nil (never had a trial), set them now
	if sub.TrialStartDate == nil {
		now := time.Now()
		sub.TrialStartDate = &now
		trialEnd := now.AddDate(0, 0, models.TrialDurationDays)
		sub.TrialEndDate = &trialEnd
	}
	// If trial dates exist, they are preserved (trial time continues from where it was)

	// Check if trial has expired
	if sub.TrialEndDate != nil && time.Now().After(*sub.TrialEndDate) {
		sub.Status = "expired"
	}

	// Clear base license specific fields
	sub.SubscriptionStartDate = nil
	sub.SubscriptionEndDate = nil

	// Update subscription
	if err := h.storage.UpdateSubscription(ctx, sub); err != nil {
		log.Printf("[REVOKE] Failed to update subscription: %v", err)
		respondError(w, http.StatusInternalServerError, "Failed to revoke license")
		return
	}

	// Clear all growth pack assignments for this tenant
	packs, _ := h.storage.GetEnabledGrowthPacks(ctx, tenantID)
	for _, pack := range packs {
		h.storage.DisableGrowthPack(ctx, tenantID, pack.PackName)
	}

	// Calculate remaining trial days
	var daysRemaining *int
	if sub.TrialEndDate != nil {
		days := int(time.Until(*sub.TrialEndDate).Hours() / 24)
		if days < 0 {
			days = 0
		}
		daysRemaining = &days
	}

	log.Printf("[REVOKE] License revoked for tenant %s, reverted to trial (%v days remaining), %d cameras need to be stopped",
		tenantID, daysRemaining, camerasToStop)

	resp := map[string]interface{}{
		"success":             true,
		"message":             "License revoked. Reverted to trial mode.",
		"plan":                sub.Plan,
		"status":              sub.Status,
		"days_remaining":      daysRemaining,
		"trial_expired":       sub.Status == "expired",
		"cameras_allowed":     models.TrialMaxCameras,
		"current_cameras":     currentCameraCount,
		"cameras_over_limit":  camerasToStop,
		"action_required":     camerasToStop > 0,
		"action_message":      getActionMessage(camerasToStop),
	}

	respondJSON(w, resp)
}

// getActionMessage returns the appropriate message based on cameras over limit
func getActionMessage(camerasToStop int) string {
	if camerasToStop == 0 {
		return ""
	}
	if camerasToStop == 1 {
		return "Please stop 1 camera to comply with trial limits."
	}
	return fmt.Sprintf("Please stop %d cameras to comply with trial limits.", camerasToStop)
}

// GetUsageSummary returns usage summary for a tenant
func (h *Handler) GetUsageSummary(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tenantID := vars["tenantId"]

	ctx := r.Context()
	log.Printf("[USAGE_SUMMARY] Request for tenant: %s", tenantID)

	// Parse query parameters for date range
	query := r.URL.Query()
	periodStart := query.Get("start")
	periodEnd := query.Get("end")

	var start, end time.Time
	var err error

	if periodStart == "" {
		start = time.Now().AddDate(0, -1, 0)
	} else {
		start, err = time.Parse(time.RFC3339, periodStart)
		if err != nil {
			start = time.Now().AddDate(0, -1, 0)
		}
	}

	if periodEnd == "" {
		end = time.Now()
	} else {
		end, err = time.Parse(time.RFC3339, periodEnd)
		if err != nil {
			end = time.Now()
		}
	}

	summary, err := h.storage.GetUsageSummary(ctx, tenantID, start, end)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to get usage summary")
		return
	}

	resp := map[string]interface{}{
		"tenant_id":    tenantID,
		"period_start": start.Format(time.RFC3339),
		"period_end":   end.Format(time.RFC3339),
	}

	// Map event types to response fields
	if v, ok := summary["api_call"]; ok {
		resp["api_calls"] = int(v)
	}
	if v, ok := summary["llm_tokens"]; ok {
		resp["llm_tokens_used"] = int(v)
	}
	if v, ok := summary["storage_gb_days"]; ok {
		resp["storage_gb_days"] = v
	}
	if v, ok := summary["sms_sent"]; ok {
		resp["sms_sent"] = int(v)
	}
	if v, ok := summary["agent_execution"]; ok {
		resp["agent_executions"] = int(v)
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

	ctx := r.Context()
	log.Printf("[CAMERA_VALIDATE] camera=%s, tenant=%s", req.CameraID, req.TenantID)

	// Use the ValidateLicense logic
	legacyReq := models.LicenseValidationRequest{
		CameraID: req.CameraID,
		TenantID: req.TenantID,
	}

	sub, _ := h.storage.GetSubscription(ctx, req.TenantID)
	packs, _ := h.storage.GetEnabledGrowthPacks(ctx, req.TenantID)

	var enabledPackNames []string
	for _, pack := range packs {
		enabledPackNames = append(enabledPackNames, pack.PackName)
	}

	isValid := true
	licenseMode := "base"
	validUntil := time.Now().AddDate(1, 0, 0)

	if sub != nil {
		licenseMode = sub.Plan
		if sub.Plan == "trial" && sub.TrialEndDate != nil {
			validUntil = *sub.TrialEndDate
			if time.Now().After(validUntil) {
				isValid = false
				licenseMode = "expired"
			}
		}
	} else {
		licenseMode = "unlicensed"
		isValid = false
	}

	// Save camera license
	if isValid && legacyReq.CameraID != "" {
		packsJSON, _ := json.Marshal(enabledPackNames)
		license := &models.CameraLicense{
			ID:                 uuid.New().String(),
			CameraID:           req.CameraID,
			TenantID:           req.TenantID,
			LicenseMode:        licenseMode,
			IsValid:            isValid,
			ValidUntil:         &validUntil,
			EnabledGrowthPacks: packsJSON,
			LastValidated:      time.Now(),
			CreatedAt:          time.Now(),
			UpdatedAt:          time.Now(),
		}
		h.storage.SaveCameraLicense(ctx, license)
	}

	resp := map[string]interface{}{
		"is_valid":             isValid,
		"license_mode":         licenseMode,
		"valid_until":          validUntil.Format(time.RFC3339),
		"enabled_growth_packs": enabledPackNames,
	}

	respondJSON(w, resp)
}

// =====================================
// Admin Endpoints
// =====================================

// CreateTenant creates a new tenant
func (h *Handler) CreateTenant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string  `json:"name"`
		Email  *string `json:"email,omitempty"`
		APIKey *string `json:"api_key,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Generate API key if not provided
	apiKey := req.APIKey
	if apiKey == nil {
		key := "bb_" + uuid.New().String()[:20]
		apiKey = &key
	}

	now := time.Now()
	tenant := &models.Tenant{
		ID:        uuid.New().String(),
		Name:      req.Name,
		Email:     req.Email,
		APIKey:    apiKey,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := h.storage.CreateTenant(ctx, tenant); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create tenant")
		return
	}

	log.Printf("[ADMIN] Created tenant: %s (%s)", tenant.ID, tenant.Name)
	respondJSON(w, tenant)
}

// UpdateTenant updates an existing tenant
func (h *Handler) UpdateTenant(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tenantID := vars["id"]

	var req struct {
		Name   *string `json:"name,omitempty"`
		Email  *string `json:"email,omitempty"`
		Status *string `json:"status,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	tenant, err := h.storage.GetTenant(ctx, tenantID)
	if err != nil || tenant == nil {
		respondError(w, http.StatusNotFound, "Tenant not found")
		return
	}

	if req.Name != nil {
		tenant.Name = *req.Name
	}
	if req.Email != nil {
		tenant.Email = req.Email
	}
	if req.Status != nil {
		tenant.Status = *req.Status
	}
	tenant.UpdatedAt = time.Now()

	if err := h.storage.UpdateTenant(ctx, tenant); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to update tenant")
		return
	}

	log.Printf("[ADMIN] Updated tenant: %s", tenantID)
	respondJSON(w, tenant)
}

// GetTenantAdmin gets a tenant by ID (admin endpoint)
func (h *Handler) GetTenantAdmin(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tenantID := vars["id"]

	ctx := r.Context()

	tenant, err := h.storage.GetTenant(ctx, tenantID)
	if err != nil || tenant == nil {
		respondError(w, http.StatusNotFound, "Tenant not found")
		return
	}

	respondJSON(w, tenant)
}

// CreateSubscription creates a new subscription for a tenant
func (h *Handler) CreateSubscription(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TenantID        string `json:"tenant_id"`
		Plan            string `json:"plan"`
		CamerasLicensed int    `json:"cameras_licensed"`
		BillingCycle    string `json:"billing_cycle"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	now := time.Now()
	var trialStart, trialEnd, subStart, subEnd *time.Time

	if req.Plan == "trial" {
		trialStart = &now
		te := now.AddDate(0, 3, 0) // 3 month trial
		trialEnd = &te
	} else {
		subStart = &now
		se := now.AddDate(1, 0, 0) // 1 year subscription
		subEnd = &se
	}

	if req.CamerasLicensed == 0 {
		if req.Plan == "trial" {
			req.CamerasLicensed = 2
		} else {
			req.CamerasLicensed = 10
		}
	}

	if req.BillingCycle == "" {
		req.BillingCycle = "monthly"
	}

	sub := &models.Subscription{
		ID:                    uuid.New().String(),
		TenantID:              req.TenantID,
		Plan:                  req.Plan,
		Status:                "active",
		CamerasLicensed:       req.CamerasLicensed,
		TrialStartDate:        trialStart,
		TrialEndDate:          trialEnd,
		SubscriptionStartDate: subStart,
		SubscriptionEndDate:   subEnd,
		BillingCycle:          req.BillingCycle,
		CreatedAt:             now,
		UpdatedAt:             now,
	}

	if err := h.storage.CreateSubscription(ctx, sub); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create subscription")
		return
	}

	log.Printf("[ADMIN] Created subscription: %s for tenant %s (plan=%s)", sub.ID, req.TenantID, req.Plan)
	respondJSON(w, sub)
}

// UpdateSubscription updates an existing subscription
func (h *Handler) UpdateSubscription(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	subID := vars["id"]

	var req struct {
		Plan            *string `json:"plan,omitempty"`
		Status          *string `json:"status,omitempty"`
		CamerasLicensed *int    `json:"cameras_licensed,omitempty"`
		BillingCycle    *string `json:"billing_cycle,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Find subscription (by querying all subscriptions - simplified)
	// In production, you'd have a GetSubscriptionByID method
	respondError(w, http.StatusNotImplemented, "Update by subscription ID not implemented, use tenant ID")
	_ = subID
	_ = ctx
}

// ManageGrowthPacks enables/disables growth packs for a tenant
func (h *Handler) ManageGrowthPacks(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tenantID := vars["tenantId"]

	var req struct {
		Enable  []string `json:"enable,omitempty"`
		Disable []string `json:"disable,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	log.Printf("[ADMIN] Managing growth packs for tenant %s: enable=%v, disable=%v",
		tenantID, req.Enable, req.Disable)

	// Get available packs for pricing
	availablePacks := models.AvailableGrowthPacks()
	packPrices := make(map[string]float64)
	for _, p := range availablePacks {
		packPrices[p.PackName] = p.PriceMonthly
	}

	// Disable packs
	for _, packName := range req.Disable {
		if err := h.storage.DisableGrowthPack(ctx, tenantID, packName); err != nil {
			log.Printf("[ADMIN] Error disabling pack %s: %v", packName, err)
		}
	}

	// Enable packs
	for _, packName := range req.Enable {
		price := packPrices[packName]
		assignment := &models.GrowthPackAssignment{
			ID:           uuid.New().String(),
			TenantID:     tenantID,
			PackName:     packName,
			EnabledAt:    time.Now(),
			IsEnabled:    true,
			PriceMonthly: &price,
		}
		if err := h.storage.EnableGrowthPack(ctx, assignment); err != nil {
			log.Printf("[ADMIN] Error enabling pack %s: %v", packName, err)
		}
	}

	// Return updated list
	packs, _ := h.storage.GetEnabledGrowthPacks(ctx, tenantID)
	var enabledPacks []string
	for _, pack := range packs {
		enabledPacks = append(enabledPacks, pack.PackName)
	}

	respondJSON(w, map[string]interface{}{
		"enabled_packs": enabledPacks,
	})
}

// =====================================
// Health & Stats
// =====================================

// HealthCheck returns server health status
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	resp := models.HealthResponse{
		Status:        "healthy",
		Service:       "brinkbyte-vision-billing",
		Version:       "2.0.0",
		Timestamp:     time.Now(),
		UptimeSeconds: time.Since(h.startTime).Seconds(),
	}

	// Get event count from stats
	ctx := r.Context()
	stats, _ := h.storage.GetStats(ctx)
	if count, ok := stats["usage_events"]; ok {
		resp.TotalEvents = count
	}

	respondJSON(w, resp)
}

// GetStats returns usage statistics
func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	stats, err := h.storage.GetStats(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to get stats")
		return
	}

	resp := models.StatsResponse{
		TotalEvents: stats["usage_events"],
		ByType:      make(map[string]int),
		Tenants:     stats["tenants"],
	}

	respondJSON(w, resp)
}

// Helper functions
func respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("[ERROR] Failed to encode JSON response: %v", err)
	}
}

func respondError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// ParseUnixTimestamp parses a Unix timestamp from string (for C++ client compatibility)
func ParseUnixTimestamp(s string) (time.Time, error) {
	// Try parsing as Unix timestamp (seconds)
	if ts, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(ts, 0), nil
	}

	// Try RFC3339
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}

	// Try other common formats
	formats := []string{
		time.RFC3339Nano,
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}

	return time.Time{}, nil
}

// init to handle deprecated package paths
func init() {
	_ = strings.TrimSpace // Use strings package to avoid import error
}
