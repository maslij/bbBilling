package models

import (
	"encoding/json"
	"time"
)

// Trial license configuration constants
const (
	TrialMaxCameras   = 2  // Maximum cameras allowed on trial
	TrialDurationDays = 90 // Trial period in days
)

// Tenant represents a customer/organization
type Tenant struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     *string   `json:"email,omitempty"`
	APIKey    *string   `json:"api_key,omitempty"`
	Status    string    `json:"status"` // active, suspended, cancelled
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Subscription represents a tenant's subscription plan
type Subscription struct {
	ID                    string     `json:"id"`
	TenantID              string     `json:"tenant_id"`
	Plan                  string     `json:"plan"` // trial, base, enterprise
	Status                string     `json:"status"` // active, cancelled, past_due, trialing
	CamerasLicensed       int        `json:"cameras_licensed"`
	TrialStartDate        *time.Time `json:"trial_start_date,omitempty"`
	TrialEndDate          *time.Time `json:"trial_end_date,omitempty"`
	SubscriptionStartDate *time.Time `json:"subscription_start_date,omitempty"`
	SubscriptionEndDate   *time.Time `json:"subscription_end_date,omitempty"`
	BillingCycle          string     `json:"billing_cycle"` // monthly, annual
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

// GrowthPackAssignment represents an enabled growth pack for a tenant
type GrowthPackAssignment struct {
	ID             string     `json:"id"`
	TenantID       string     `json:"tenant_id"`
	SubscriptionID *string    `json:"subscription_id,omitempty"`
	PackName       string     `json:"pack_name"`
	EnabledAt      time.Time  `json:"enabled_at"`
	DisabledAt     *time.Time `json:"disabled_at,omitempty"`
	IsEnabled      bool       `json:"is_enabled"`
	PriceMonthly   *float64   `json:"price_monthly,omitempty"`
}

// CameraLicense represents a camera's license status
type CameraLicense struct {
	ID                 string          `json:"id"`
	CameraID           string          `json:"camera_id"`
	TenantID           string          `json:"tenant_id"`
	DeviceID           *string         `json:"device_id,omitempty"`
	LicenseMode        string          `json:"license_mode"` // trial, base, unlicensed
	IsValid            bool            `json:"is_valid"`
	ValidUntil         *time.Time      `json:"valid_until,omitempty"`
	EnabledGrowthPacks json.RawMessage `json:"enabled_growth_packs"`
	LastValidated      time.Time       `json:"last_validated"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

// FeatureEntitlement represents a feature access entitlement for a tenant
type FeatureEntitlement struct {
	ID              string     `json:"id"`
	TenantID        string     `json:"tenant_id"`
	FeatureCategory string     `json:"feature_category"` // cv_models, analytics, outputs, agents, llm
	FeatureName     string     `json:"feature_name"`
	IsEnabled       bool       `json:"is_enabled"`
	QuotaLimit      int        `json:"quota_limit"` // -1 for unlimited
	QuotaUsed       int        `json:"quota_used"`
	ValidUntil      *time.Time `json:"valid_until,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// EdgeDevice represents an edge device registered with the billing system
type EdgeDevice struct {
	ID                string     `json:"id"`
	DeviceID          string     `json:"device_id"`
	TenantID          string     `json:"tenant_id"`
	Name              *string    `json:"name,omitempty"`
	Status            string     `json:"status"` // active, offline, suspended
	ManagementTier    string     `json:"management_tier"` // basic, managed
	LastHeartbeat     *time.Time `json:"last_heartbeat,omitempty"`
	ActiveCameraCount int        `json:"active_camera_count"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// GrowthPackInfo represents growth pack metadata
type GrowthPackInfo struct {
	PackID       string   `json:"pack_id"`
	PackName     string   `json:"pack_name"`
	Description  string   `json:"description"`
	Category     string   `json:"category"`
	PriceMonthly float64  `json:"price_monthly"`
	Features     []string `json:"features"`
	IsEnabled    bool     `json:"is_enabled"`
}

// AvailableGrowthPacks returns all available growth packs with their details
// Prices are in AUD
func AvailableGrowthPacks() []GrowthPackInfo {
	return []GrowthPackInfo{
		{
			PackID:       "pack-advanced-analytics",
			PackName:     "Advanced Analytics",
			Description:  "Advanced analytics and reporting features",
			Category:     "analytics",
			PriceMonthly: 29.00,
			Features: []string{
				"Near-miss detection",
				"Interaction time tracking",
				"Queue counting",
				"Object size estimation",
			},
		},
		{
			PackID:       "pack-active-transport",
			PackName:     "Active Transport",
			Description:  "Active transport mode detection and analytics",
			Category:     "intelligence",
			PriceMonthly: 45.00,
			Features: []string{
				"Bicycle detection",
				"Scooter detection",
				"Pram/stroller detection",
				"Wheelchair detection",
			},
		},
		{
			PackID:       "pack-cloud-storage",
			PackName:     "Cloud Storage",
			Description:  "Extended cloud storage for video and analytics data",
			Category:     "data",
			PriceMonthly: 149.00,
			Features: []string{
				"1TB cloud storage",
				"30-day retention",
				"Encrypted backups",
				"High-availability storage",
			},
		},
		{
			PackID:       "pack-api-integration",
			PackName:     "API Integration",
			Description:  "Advanced API access and integration capabilities",
			Category:     "integration",
			PriceMonthly: 109.00,
			Features: []string{
				"Unlimited API calls",
				"Webhook support",
				"Custom integrations",
				"Priority support",
			},
		},
		{
			PackID:       "pack-intelligence",
			PackName:     "Intelligence",
			Description:  "AI-powered insights and LLM-based analytics",
			Category:     "intelligence",
			PriceMonthly: 599.00,
			Features: []string{
				"Full analyst seat",
				"Premium connectors",
				"Automated reports",
				"Natural language queries",
			},
		},
		{
			PackID:       "pack-emergency-vehicles",
			PackName:     "Emergency Vehicles",
			Description:  "Emergency vehicle detection for traffic management",
			Category:     "industry",
			PriceMonthly: 39.00,
			Features: []string{
				"Police vehicle detection",
				"Ambulance detection",
				"Fire truck detection",
			},
		},
		{
			PackID:       "pack-retail",
			PackName:     "Retail",
			Description:  "Retail-specific detection and analytics",
			Category:     "industry",
			PriceMonthly: 49.00,
			Features: []string{
				"Shopping trolley detection",
				"Staff detection",
				"Customer flow analysis",
			},
		},
	}
}

// BaseFeatures returns features included with base license
func BaseFeatures() map[string][]string {
	return map[string][]string{
		"cv_models": {
			"person", "car", "van", "truck", "bus", "motorcycle",
		},
		"analytics": {
			"detection", "tracking", "counting", "dwell", "heatmap",
			"direction", "speed", "privacy_mask",
		},
		"outputs": {
			"edge_io", "dashboard", "email", "webhook", "api",
		},
	}
}

// GetGrowthPackFeatures returns features for a specific growth pack
func GetGrowthPackFeatures(packName string) map[string][]string {
	packFeatures := map[string]map[string][]string{
		"Advanced Analytics": {
			"analytics": {"near_miss", "interaction_time", "queue_counter", "object_size"},
		},
		"Active Transport": {
			"cv_models": {"bike", "scooter", "pram", "wheelchair"},
		},
		"Cloud Storage": {
			"outputs": {"cloud_backup", "extended_retention", "encrypted_storage"},
		},
		"API Integration": {
			"outputs": {"unlimited_api", "webhooks", "custom_integrations", "priority_support"},
		},
		"Intelligence": {
			"llm": {"analyst_seat_full", "premium_connectors", "automated_reports"},
		},
		"Emergency Vehicles": {
			"cv_models": {"police", "ambulance", "fire_fighter"},
		},
		"Retail": {
			"cv_models": {"trolley", "staff", "customer"},
		},
	}

	if features, ok := packFeatures[packName]; ok {
		return features
	}
	return nil
}

