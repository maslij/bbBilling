package models

import "time"

// License validation structures
type LicenseValidationRequest struct {
	CameraID string `json:"camera_id"`
	TenantID string `json:"tenant_id"`
	DeviceID string `json:"device_id"`
}

type LicenseValidationResponse struct {
	IsValid            bool      `json:"is_valid"`
	LicenseMode        string    `json:"license_mode"`
	EnabledGrowthPacks []string  `json:"enabled_growth_packs"`
	ValidUntil         time.Time `json:"valid_until"`
	CamerasAllowed     int       `json:"cameras_allowed"`
}

// Entitlement check structures
type EntitlementCheckRequest struct {
	TenantID        string `json:"tenant_id"`
	FeatureCategory string `json:"feature_category"`
	FeatureName     string `json:"feature_name"`
}

type EntitlementCheckResponse struct {
	IsEnabled      bool      `json:"is_enabled"`
	QuotaRemaining int       `json:"quota_remaining"`
	ValidUntil     time.Time `json:"valid_until"`
}

// Usage reporting structures
type UsageEvent struct {
	TenantID   string                 `json:"tenant_id"`
	EventType  string                 `json:"event_type"`
	ResourceID string                 `json:"resource_id"`
	Quantity   float64                `json:"quantity"`
	Unit       string                 `json:"unit"`
	EventTime  time.Time              `json:"event_time"`
	Metadata   map[string]interface{} `json:"metadata"`
}

type UsageBatchRequest struct {
	Events []UsageEvent `json:"events"`
}

type UsageBatchResponse struct {
	AcceptedCount int      `json:"accepted_count"`
	RejectedCount int      `json:"rejected_count"`
	Errors        []string `json:"errors"`
}

// Heartbeat structures
type HeartbeatRequest struct {
	DeviceID        string   `json:"device_id"`
	TenantID        string   `json:"tenant_id"`
	ActiveCameraIDs []string `json:"active_camera_ids"`
	ManagementTier  string   `json:"management_tier"`
}

type HeartbeatResponse struct {
	Status               string `json:"status"`
	NextHeartbeatSeconds int    `json:"next_heartbeat_in_seconds"`
}

// Health check response
type HealthResponse struct {
	Status        string    `json:"status"`
	Service       string    `json:"service"`
	Version       string    `json:"version"`
	Timestamp     time.Time `json:"timestamp"`
	UptimeSeconds float64   `json:"uptime_seconds"`
	TotalEvents   int       `json:"total_events"`
}

// Stats response
type StatsResponse struct {
	TotalEvents int            `json:"total_events"`
	ByType      map[string]int `json:"by_type"`
	Tenants     int            `json:"tenants"`
}

