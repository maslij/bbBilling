package storage

import (
	"sync"
	"brinkbyte-billing-server/models"
)

// InMemoryStorage provides thread-safe in-memory storage for the billing server
// In production, this would be replaced with a real database
type InMemoryStorage struct {
	tenants      map[string]map[string]interface{}
	usageEvents  []models.UsageEvent
	licenses     map[string]*models.LicenseValidationResponse
	mu           sync.RWMutex
}

// NewInMemoryStorage creates a new storage instance
func NewInMemoryStorage() *InMemoryStorage {
	return &InMemoryStorage{
		tenants:     make(map[string]map[string]interface{}),
		usageEvents: make([]models.UsageEvent, 0),
		licenses:    make(map[string]*models.LicenseValidationResponse),
	}
}

// AddUsageEvents stores usage events
func (s *InMemoryStorage) AddUsageEvents(events []models.UsageEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usageEvents = append(s.usageEvents, events...)
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

// StoreLicense stores a license for a camera
func (s *InMemoryStorage) StoreLicense(cameraID string, license *models.LicenseValidationResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.licenses[cameraID] = license
}

// GetLicense retrieves a license for a camera
func (s *InMemoryStorage) GetLicense(cameraID string) (*models.LicenseValidationResponse, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	license, exists := s.licenses[cameraID]
	return license, exists
}

