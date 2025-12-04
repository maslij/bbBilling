package models

import (
	"encoding/json"
	"strconv"
	"time"
)

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

// UsageEvent represents a usage event for storage
type UsageEvent struct {
	TenantID   string          `json:"tenant_id"`
	EventType  string          `json:"event_type"`
	ResourceID string          `json:"resource_id"`
	Quantity   float64         `json:"quantity"`
	Unit       string          `json:"unit"`
	EventTime  time.Time       `json:"event_time"`
	Metadata   json.RawMessage `json:"metadata"`
}

// UsageEventLegacy is the legacy format from C++ client with flexible event_time
type UsageEventLegacy struct {
	TenantID   string                 `json:"tenant_id"`
	EventType  string                 `json:"event_type"`
	ResourceID string                 `json:"resource_id"`
	Quantity   float64                `json:"quantity"`
	Unit       string                 `json:"unit"`
	EventTime  time.Time              `json:"event_time"`
	Metadata   map[string]interface{} `json:"metadata"`
}

// FlexibleTime handles time that may come as Unix timestamp string or RFC3339
type FlexibleTime struct {
	time.Time
}

func (ft *FlexibleTime) UnmarshalJSON(data []byte) error {
	// Remove quotes if present
	s := string(data)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}

	// Try parsing as Unix timestamp (seconds)
	if ts, err := strconv.ParseInt(s, 10, 64); err == nil {
		ft.Time = time.Unix(ts, 0)
		return nil
	}

	// Try RFC3339
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		ft.Time = t
		return nil
	}

	// Try RFC3339Nano
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		ft.Time = t
		return nil
	}

	// Try other formats
	formats := []string{
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			ft.Time = t
			return nil
		}
	}

	// Default to current time if all parsing fails
	ft.Time = time.Now()
	return nil
}

// UsageEventInput is the input format from C++ client with flexible event_time
type UsageEventInput struct {
	TenantID   string                 `json:"tenant_id"`
	EventType  string                 `json:"event_type"`
	ResourceID string                 `json:"resource_id"`
	Quantity   float64                `json:"quantity"`
	Unit       string                 `json:"unit"`
	EventTime  FlexibleTime           `json:"event_time"`
	Metadata   map[string]interface{} `json:"metadata"`
}

// UsageBatchRequest is the input from clients
type UsageBatchRequest struct {
	Events []UsageEventInput `json:"events"`
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
