package storage

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"brinkbyte-billing-server/models"
)

// InMemoryStorage provides thread-safe in-memory storage for the billing server
// This is used for development/testing when PostgreSQL is not available
type InMemoryStorage struct {
	tenants       map[string]*models.Tenant
	subscriptions map[string]*models.Subscription // keyed by tenant_id
	growthPacks   map[string][]models.GrowthPackAssignment // keyed by tenant_id
	cameras       map[string]*models.CameraLicense // keyed by "tenantId:cameraId"
	entitlements  map[string]*models.FeatureEntitlement // keyed by "tenantId:category:feature"
	usageEvents   []models.UsageEvent
	devices       map[string]*models.EdgeDevice // keyed by device_id
	mu            sync.RWMutex
}

// NewInMemoryStorage creates a new storage instance
func NewInMemoryStorage() *InMemoryStorage {
	return &InMemoryStorage{
		tenants:       make(map[string]*models.Tenant),
		subscriptions: make(map[string]*models.Subscription),
		growthPacks:   make(map[string][]models.GrowthPackAssignment),
		cameras:       make(map[string]*models.CameraLicense),
		entitlements:  make(map[string]*models.FeatureEntitlement),
		usageEvents:   make([]models.UsageEvent, 0),
		devices:       make(map[string]*models.EdgeDevice),
	}
}

// =====================================
// Tenant Operations
// =====================================

func (s *InMemoryStorage) GetTenant(ctx context.Context, tenantID string) (*models.Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if t, ok := s.tenants[tenantID]; ok {
		return t, nil
	}
	return nil, nil
}

func (s *InMemoryStorage) GetTenantByAPIKey(ctx context.Context, apiKey string) (*models.Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.tenants {
		if t.APIKey != nil && *t.APIKey == apiKey {
			return t, nil
		}
	}
	return nil, nil
}

func (s *InMemoryStorage) CreateTenant(ctx context.Context, tenant *models.Tenant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tenants[tenant.ID] = tenant
	return nil
}

func (s *InMemoryStorage) UpdateTenant(ctx context.Context, tenant *models.Tenant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tenants[tenant.ID] = tenant
	return nil
}

// =====================================
// Subscription Operations
// =====================================

func (s *InMemoryStorage) GetSubscription(ctx context.Context, tenantID string) (*models.Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if sub, ok := s.subscriptions[tenantID]; ok {
		return sub, nil
	}
	return nil, nil
}

func (s *InMemoryStorage) CreateSubscription(ctx context.Context, sub *models.Subscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscriptions[sub.TenantID] = sub
	return nil
}

func (s *InMemoryStorage) UpdateSubscription(ctx context.Context, sub *models.Subscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscriptions[sub.TenantID] = sub
	return nil
}

// =====================================
// Growth Pack Operations
// =====================================

func (s *InMemoryStorage) GetEnabledGrowthPacks(ctx context.Context, tenantID string) ([]models.GrowthPackAssignment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	packs := s.growthPacks[tenantID]
	var enabled []models.GrowthPackAssignment
	for _, p := range packs {
		if p.IsEnabled {
			enabled = append(enabled, p)
		}
	}
	return enabled, nil
}

func (s *InMemoryStorage) EnableGrowthPack(ctx context.Context, assignment *models.GrowthPackAssignment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	packs := s.growthPacks[assignment.TenantID]
	
	// Check if pack already exists
	for i, p := range packs {
		if p.PackName == assignment.PackName {
			packs[i].IsEnabled = true
			packs[i].EnabledAt = assignment.EnabledAt
			packs[i].DisabledAt = nil
			s.growthPacks[assignment.TenantID] = packs
			return nil
	}
}

	// Add new pack
	s.growthPacks[assignment.TenantID] = append(packs, *assignment)
	return nil
}

func (s *InMemoryStorage) DisableGrowthPack(ctx context.Context, tenantID, packName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	packs := s.growthPacks[tenantID]
	for i, p := range packs {
		if p.PackName == packName {
			now := time.Now()
			packs[i].IsEnabled = false
			packs[i].DisabledAt = &now
			s.growthPacks[tenantID] = packs
			return nil
		}
	}
	return nil
}

// =====================================
// Camera License Operations
// =====================================

func cameraKey(cameraID, tenantID string) string {
	return tenantID + ":" + cameraID
}

func (s *InMemoryStorage) GetCameraLicense(ctx context.Context, cameraID, tenantID string) (*models.CameraLicense, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if cam, ok := s.cameras[cameraKey(cameraID, tenantID)]; ok {
		return cam, nil
	}
	return nil, nil
}

func (s *InMemoryStorage) SaveCameraLicense(ctx context.Context, license *models.CameraLicense) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cameras[cameraKey(license.CameraID, license.TenantID)] = license
	return nil
}

func (s *InMemoryStorage) GetCamerasByTenant(ctx context.Context, tenantID string) ([]models.CameraLicense, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	var cameras []models.CameraLicense
	for key, cam := range s.cameras {
		if len(key) > len(tenantID) && key[:len(tenantID)] == tenantID {
			cameras = append(cameras, *cam)
		}
	}
	return cameras, nil
}

func (s *InMemoryStorage) CountCamerasByTenant(ctx context.Context, tenantID string) (int, error) {
	cameras, _ := s.GetCamerasByTenant(ctx, tenantID)
	return len(cameras), nil
}

// =====================================
// Entitlement Operations
// =====================================

func entitlementKey(tenantID, category, feature string) string {
	return tenantID + ":" + category + ":" + feature
}

func (s *InMemoryStorage) GetEntitlement(ctx context.Context, tenantID, category, feature string) (*models.FeatureEntitlement, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if ent, ok := s.entitlements[entitlementKey(tenantID, category, feature)]; ok {
		return ent, nil
	}
	return nil, nil
}

func (s *InMemoryStorage) SaveEntitlement(ctx context.Context, ent *models.FeatureEntitlement) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entitlements[entitlementKey(ent.TenantID, ent.FeatureCategory, ent.FeatureName)] = ent
	return nil
}

// =====================================
// Usage Event Operations
// =====================================

func (s *InMemoryStorage) SaveUsageEvents(ctx context.Context, events []models.UsageEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usageEvents = append(s.usageEvents, events...)
	return nil
}

func (s *InMemoryStorage) GetUsageSummary(ctx context.Context, tenantID string, start, end time.Time) (map[string]float64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	summary := make(map[string]float64)
	for _, event := range s.usageEvents {
		if event.TenantID == tenantID && 
		   (event.EventTime.After(start) || event.EventTime.Equal(start)) &&
		   (event.EventTime.Before(end) || event.EventTime.Equal(end)) {
			summary[event.EventType] += event.Quantity
		}
	}
	return summary, nil
}

// =====================================
// Edge Device Operations
// =====================================

func (s *InMemoryStorage) SaveEdgeDevice(ctx context.Context, device *models.EdgeDevice) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.devices[device.DeviceID] = device
	return nil
}

func (s *InMemoryStorage) GetEdgeDevice(ctx context.Context, deviceID string) (*models.EdgeDevice, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if d, ok := s.devices[deviceID]; ok {
		return d, nil
	}
	return nil, nil
}

// =====================================
// Statistics
// =====================================

func (s *InMemoryStorage) GetStats(ctx context.Context) (map[string]int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	stats := map[string]int{
		"tenants":      len(s.tenants),
		"usage_events": len(s.usageEvents),
		"cameras":      len(s.cameras),
		"devices":      len(s.devices),
	}
	return stats, nil
}

// =====================================
// Legacy methods for backwards compatibility
// =====================================

// AddUsageEvents stores usage events (legacy)
func (s *InMemoryStorage) AddUsageEvents(events []models.UsageEventLegacy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	for _, e := range events {
		metadata, _ := json.Marshal(e.Metadata)
		s.usageEvents = append(s.usageEvents, models.UsageEvent{
			TenantID:   e.TenantID,
			EventType:  e.EventType,
			ResourceID: e.ResourceID,
			Quantity:   e.Quantity,
			Unit:       e.Unit,
			Metadata:   metadata,
			EventTime:  e.EventTime,
		})
	}
}

// GetUsageEventCount returns the total number of usage events
func (s *InMemoryStorage) GetUsageEventCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.usageEvents)
}

// GetUsageStats returns usage statistics by type
func (s *InMemoryStorage) GetUsageStats() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	stats := make(map[string]int)
	for _, event := range s.usageEvents {
		stats[event.EventType]++
	}
	return stats
}

// GetTenantCount returns the number of unique tenants
func (s *InMemoryStorage) GetTenantCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.tenants)
}

// StoreLicense stores a license for a camera (legacy)
func (s *InMemoryStorage) StoreLicense(cameraID string, license *models.LicenseValidationResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	packsJSON, _ := json.Marshal(license.EnabledGrowthPacks)
	cam := &models.CameraLicense{
		ID:                 cameraID,
		CameraID:           cameraID,
		LicenseMode:        license.LicenseMode,
		IsValid:            license.IsValid,
		ValidUntil:         &license.ValidUntil,
		EnabledGrowthPacks: packsJSON,
		LastValidated:      time.Now(),
	}
	s.cameras[cameraID] = cam
}

// GetLicense retrieves a license for a camera (legacy)
func (s *InMemoryStorage) GetLicense(cameraID string) (*models.LicenseValidationResponse, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if cam, ok := s.cameras[cameraID]; ok {
		var packs []string
		json.Unmarshal(cam.EnabledGrowthPacks, &packs)
		
		validUntil := time.Now().AddDate(1, 0, 0)
		if cam.ValidUntil != nil {
			validUntil = *cam.ValidUntil
		}
		
		return &models.LicenseValidationResponse{
			IsValid:            cam.IsValid,
			LicenseMode:        cam.LicenseMode,
			EnabledGrowthPacks: packs,
			ValidUntil:         validUntil,
		}, true
	}
	return nil, false
}
