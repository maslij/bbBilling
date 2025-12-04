package storage

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"

	"brinkbyte-billing-server/models"
)

// PostgresStorage provides persistent storage using PostgreSQL
type PostgresStorage struct {
	pool *pgxpool.Pool
}

// PostgresConfig holds database connection configuration
type PostgresConfig struct {
	Host     string
	Port     int
	Database string
	User     string
	Password string
	PoolSize int
}

// LoadPostgresConfigFromEnv loads PostgreSQL configuration from environment variables
func LoadPostgresConfigFromEnv() PostgresConfig {
	port := 5432
	if p := os.Getenv("POSTGRES_PORT"); p != "" {
		fmt.Sscanf(p, "%d", &port)
	}

	poolSize := 10
	if ps := os.Getenv("POSTGRES_POOL_SIZE"); ps != "" {
		fmt.Sscanf(ps, "%d", &poolSize)
	}

	return PostgresConfig{
		Host:     getEnvOrDefault("POSTGRES_HOST", "localhost"),
		Port:     port,
		Database: getEnvOrDefault("POSTGRES_DATABASE", "billing"),
		User:     getEnvOrDefault("POSTGRES_USER", "billing_user"),
		Password: getEnvOrDefault("POSTGRES_PASSWORD", "billing_password"),
		PoolSize: poolSize,
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}

// NewPostgresStorage creates a new PostgreSQL storage instance
func NewPostgresStorage(ctx context.Context, config PostgresConfig) (*PostgresStorage, error) {
	connStr := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?pool_max_conns=%d",
		config.User, config.Password, config.Host, config.Port, config.Database, config.PoolSize,
	)

	poolConfig, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	poolConfig.MaxConns = int32(config.PoolSize)
	poolConfig.MinConns = 2
	poolConfig.MaxConnLifetime = time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.ConnectConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Printf("[POSTGRES] Connected to %s:%d/%s (pool_size=%d)", config.Host, config.Port, config.Database, config.PoolSize)

	storage := &PostgresStorage{pool: pool}

	// Initialize schema
	if err := storage.initSchema(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return storage, nil
}

// Close closes the database connection pool
func (s *PostgresStorage) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

// initSchema creates the required tables if they don't exist
func (s *PostgresStorage) initSchema(ctx context.Context) error {
	schema := `
	-- Tenants table (using TEXT for tenant_id to support string identifiers like "default")
	CREATE TABLE IF NOT EXISTS tenants (
		id TEXT PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		email VARCHAR(255),
		api_key VARCHAR(255) UNIQUE,
		status VARCHAR(50) DEFAULT 'active',
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	-- Subscriptions table
	CREATE TABLE IF NOT EXISTS subscriptions (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		plan VARCHAR(50) NOT NULL DEFAULT 'trial',
		status VARCHAR(50) DEFAULT 'active',
		cameras_licensed INTEGER DEFAULT 2,
		trial_start_date TIMESTAMP WITH TIME ZONE,
		trial_end_date TIMESTAMP WITH TIME ZONE,
		subscription_start_date TIMESTAMP WITH TIME ZONE,
		subscription_end_date TIMESTAMP WITH TIME ZONE,
		billing_cycle VARCHAR(50) DEFAULT 'monthly',
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	-- Growth pack assignments
	CREATE TABLE IF NOT EXISTS growth_pack_assignments (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		subscription_id TEXT REFERENCES subscriptions(id),
		pack_name VARCHAR(100) NOT NULL,
		enabled_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		disabled_at TIMESTAMP WITH TIME ZONE,
		is_enabled BOOLEAN DEFAULT true,
		price_monthly DECIMAL(10,2),
		UNIQUE(tenant_id, pack_name)
	);

	-- Camera licenses (cached from edge devices)
	CREATE TABLE IF NOT EXISTS camera_licenses (
		id TEXT PRIMARY KEY,
		camera_id VARCHAR(255) NOT NULL,
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		device_id VARCHAR(255),
		license_mode VARCHAR(50) DEFAULT 'trial',
		is_valid BOOLEAN DEFAULT true,
		valid_until TIMESTAMP WITH TIME ZONE,
		enabled_growth_packs JSONB DEFAULT '[]',
		last_validated TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(camera_id, tenant_id)
	);

	-- Feature entitlements
	CREATE TABLE IF NOT EXISTS feature_entitlements (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		feature_category VARCHAR(100) NOT NULL,
		feature_name VARCHAR(255) NOT NULL,
		is_enabled BOOLEAN DEFAULT false,
		quota_limit INTEGER DEFAULT -1,
		quota_used INTEGER DEFAULT 0,
		valid_until TIMESTAMP WITH TIME ZONE,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(tenant_id, feature_category, feature_name)
	);

	-- Usage events
	CREATE TABLE IF NOT EXISTS usage_events (
		id BIGSERIAL PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		event_type VARCHAR(100) NOT NULL,
		resource_id VARCHAR(255),
		quantity DECIMAL(15,5) NOT NULL DEFAULT 1,
		unit VARCHAR(50) NOT NULL,
		metadata JSONB DEFAULT '{}',
		event_time TIMESTAMP WITH TIME ZONE NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	-- API keys table
	CREATE TABLE IF NOT EXISTS api_keys (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		key_hash VARCHAR(255) NOT NULL UNIQUE,
		key_prefix VARCHAR(10) NOT NULL,
		name VARCHAR(255),
		is_active BOOLEAN DEFAULT true,
		last_used_at TIMESTAMP WITH TIME ZONE,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMP WITH TIME ZONE
	);

	-- Edge devices
	CREATE TABLE IF NOT EXISTS edge_devices (
		id TEXT PRIMARY KEY,
		device_id VARCHAR(255) UNIQUE NOT NULL,
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		name VARCHAR(255),
		status VARCHAR(50) DEFAULT 'active',
		management_tier VARCHAR(50) DEFAULT 'basic',
		last_heartbeat TIMESTAMP WITH TIME ZONE,
		active_camera_count INTEGER DEFAULT 0,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	-- Create indexes
	CREATE INDEX IF NOT EXISTS idx_subscriptions_tenant ON subscriptions(tenant_id);
	CREATE INDEX IF NOT EXISTS idx_growth_packs_tenant ON growth_pack_assignments(tenant_id);
	CREATE INDEX IF NOT EXISTS idx_camera_licenses_tenant ON camera_licenses(tenant_id);
	CREATE INDEX IF NOT EXISTS idx_camera_licenses_camera ON camera_licenses(camera_id);
	CREATE INDEX IF NOT EXISTS idx_usage_events_tenant ON usage_events(tenant_id, event_time);
	CREATE INDEX IF NOT EXISTS idx_usage_events_type ON usage_events(event_type, event_time);
	CREATE INDEX IF NOT EXISTS idx_api_keys_tenant ON api_keys(tenant_id);
	CREATE INDEX IF NOT EXISTS idx_edge_devices_tenant ON edge_devices(tenant_id);
	`

	_, err := s.pool.Exec(ctx, schema)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	log.Printf("[POSTGRES] Schema initialized successfully")
	return nil
}

// =====================================
// Tenant Operations
// =====================================

// GetTenant retrieves a tenant by ID
func (s *PostgresStorage) GetTenant(ctx context.Context, tenantID string) (*models.Tenant, error) {
	query := `
		SELECT id, name, email, api_key, status, created_at, updated_at
		FROM tenants WHERE id = $1
	`

	var tenant models.Tenant
	err := s.pool.QueryRow(ctx, query, tenantID).Scan(
		&tenant.ID, &tenant.Name, &tenant.Email, &tenant.APIKey,
		&tenant.Status, &tenant.CreatedAt, &tenant.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}

	return &tenant, nil
}

// GetTenantByAPIKey retrieves a tenant by API key
func (s *PostgresStorage) GetTenantByAPIKey(ctx context.Context, apiKey string) (*models.Tenant, error) {
	query := `
		SELECT id, name, email, api_key, status, created_at, updated_at
		FROM tenants WHERE api_key = $1
	`

	var tenant models.Tenant
	err := s.pool.QueryRow(ctx, query, apiKey).Scan(
		&tenant.ID, &tenant.Name, &tenant.Email, &tenant.APIKey,
		&tenant.Status, &tenant.CreatedAt, &tenant.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant by API key: %w", err)
	}

	return &tenant, nil
}

// CreateTenant creates a new tenant
func (s *PostgresStorage) CreateTenant(ctx context.Context, tenant *models.Tenant) error {
	query := `
		INSERT INTO tenants (id, name, email, api_key, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err := s.pool.Exec(ctx, query,
		tenant.ID, tenant.Name, tenant.Email, tenant.APIKey,
		tenant.Status, tenant.CreatedAt, tenant.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create tenant: %w", err)
	}

	return nil
}

// UpdateTenant updates an existing tenant
func (s *PostgresStorage) UpdateTenant(ctx context.Context, tenant *models.Tenant) error {
	query := `
		UPDATE tenants SET name = $2, email = $3, status = $4, updated_at = $5
		WHERE id = $1
	`

	_, err := s.pool.Exec(ctx, query,
		tenant.ID, tenant.Name, tenant.Email, tenant.Status, time.Now(),
	)
	if err != nil {
		return fmt.Errorf("failed to update tenant: %w", err)
	}

	return nil
}

// =====================================
// Subscription Operations
// =====================================

// GetSubscription retrieves subscription for a tenant
func (s *PostgresStorage) GetSubscription(ctx context.Context, tenantID string) (*models.Subscription, error) {
	query := `
		SELECT id, tenant_id, plan, status, cameras_licensed,
			   trial_start_date, trial_end_date, subscription_start_date, subscription_end_date,
			   billing_cycle, created_at, updated_at
		FROM subscriptions WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT 1
	`

	var sub models.Subscription
	err := s.pool.QueryRow(ctx, query, tenantID).Scan(
		&sub.ID, &sub.TenantID, &sub.Plan, &sub.Status, &sub.CamerasLicensed,
		&sub.TrialStartDate, &sub.TrialEndDate, &sub.SubscriptionStartDate, &sub.SubscriptionEndDate,
		&sub.BillingCycle, &sub.CreatedAt, &sub.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	return &sub, nil
}

// CreateSubscription creates a new subscription
func (s *PostgresStorage) CreateSubscription(ctx context.Context, sub *models.Subscription) error {
	query := `
		INSERT INTO subscriptions (id, tenant_id, plan, status, cameras_licensed,
			trial_start_date, trial_end_date, subscription_start_date, subscription_end_date,
			billing_cycle, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`

	_, err := s.pool.Exec(ctx, query,
		sub.ID, sub.TenantID, sub.Plan, sub.Status, sub.CamerasLicensed,
		sub.TrialStartDate, sub.TrialEndDate, sub.SubscriptionStartDate, sub.SubscriptionEndDate,
		sub.BillingCycle, sub.CreatedAt, sub.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create subscription: %w", err)
	}

	return nil
}

// UpdateSubscription updates an existing subscription
func (s *PostgresStorage) UpdateSubscription(ctx context.Context, sub *models.Subscription) error {
	query := `
		UPDATE subscriptions SET plan = $2, status = $3, cameras_licensed = $4,
			trial_end_date = $5, subscription_end_date = $6, billing_cycle = $7, updated_at = $8
		WHERE id = $1
	`

	_, err := s.pool.Exec(ctx, query,
		sub.ID, sub.Plan, sub.Status, sub.CamerasLicensed,
		sub.TrialEndDate, sub.SubscriptionEndDate, sub.BillingCycle, time.Now(),
	)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	return nil
}

// =====================================
// Growth Pack Operations
// =====================================

// GetEnabledGrowthPacks gets all enabled growth packs for a tenant
func (s *PostgresStorage) GetEnabledGrowthPacks(ctx context.Context, tenantID string) ([]models.GrowthPackAssignment, error) {
	query := `
		SELECT id, tenant_id, subscription_id, pack_name, enabled_at, disabled_at, is_enabled, price_monthly
		FROM growth_pack_assignments
		WHERE tenant_id = $1 AND is_enabled = true
	`

	rows, err := s.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get growth packs: %w", err)
	}
	defer rows.Close()

	var packs []models.GrowthPackAssignment
	for rows.Next() {
		var pack models.GrowthPackAssignment
		err := rows.Scan(
			&pack.ID, &pack.TenantID, &pack.SubscriptionID, &pack.PackName,
			&pack.EnabledAt, &pack.DisabledAt, &pack.IsEnabled, &pack.PriceMonthly,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan growth pack: %w", err)
		}
		packs = append(packs, pack)
	}

	return packs, nil
}

// EnableGrowthPack enables a growth pack for a tenant
func (s *PostgresStorage) EnableGrowthPack(ctx context.Context, assignment *models.GrowthPackAssignment) error {
	query := `
		INSERT INTO growth_pack_assignments (id, tenant_id, subscription_id, pack_name, enabled_at, is_enabled, price_monthly)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (tenant_id, pack_name) DO UPDATE SET
			is_enabled = true, enabled_at = EXCLUDED.enabled_at, disabled_at = NULL, price_monthly = EXCLUDED.price_monthly
	`

	_, err := s.pool.Exec(ctx, query,
		assignment.ID, assignment.TenantID, assignment.SubscriptionID, assignment.PackName,
		assignment.EnabledAt, assignment.IsEnabled, assignment.PriceMonthly,
	)
	if err != nil {
		return fmt.Errorf("failed to enable growth pack: %w", err)
	}

	return nil
}

// DisableGrowthPack disables a growth pack for a tenant
func (s *PostgresStorage) DisableGrowthPack(ctx context.Context, tenantID, packName string) error {
	query := `
		UPDATE growth_pack_assignments SET is_enabled = false, disabled_at = $3
		WHERE tenant_id = $1 AND pack_name = $2
	`

	_, err := s.pool.Exec(ctx, query, tenantID, packName, time.Now())
	if err != nil {
		return fmt.Errorf("failed to disable growth pack: %w", err)
	}

	return nil
}

// =====================================
// Camera License Operations
// =====================================

// GetCameraLicense retrieves a camera license
func (s *PostgresStorage) GetCameraLicense(ctx context.Context, cameraID, tenantID string) (*models.CameraLicense, error) {
	query := `
		SELECT id, camera_id, tenant_id, device_id, license_mode, is_valid, valid_until,
			   enabled_growth_packs, last_validated, created_at, updated_at
		FROM camera_licenses WHERE camera_id = $1 AND tenant_id = $2
	`

	var license models.CameraLicense
	err := s.pool.QueryRow(ctx, query, cameraID, tenantID).Scan(
		&license.ID, &license.CameraID, &license.TenantID, &license.DeviceID,
		&license.LicenseMode, &license.IsValid, &license.ValidUntil,
		&license.EnabledGrowthPacks, &license.LastValidated, &license.CreatedAt, &license.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get camera license: %w", err)
	}

	return &license, nil
}

// SaveCameraLicense creates or updates a camera license
func (s *PostgresStorage) SaveCameraLicense(ctx context.Context, license *models.CameraLicense) error {
	query := `
		INSERT INTO camera_licenses (id, camera_id, tenant_id, device_id, license_mode, is_valid,
			valid_until, enabled_growth_packs, last_validated, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (camera_id, tenant_id) DO UPDATE SET
			device_id = EXCLUDED.device_id, license_mode = EXCLUDED.license_mode, is_valid = EXCLUDED.is_valid,
			valid_until = EXCLUDED.valid_until, enabled_growth_packs = EXCLUDED.enabled_growth_packs,
			last_validated = EXCLUDED.last_validated, updated_at = EXCLUDED.updated_at
	`

	_, err := s.pool.Exec(ctx, query,
		license.ID, license.CameraID, license.TenantID, license.DeviceID,
		license.LicenseMode, license.IsValid, license.ValidUntil,
		license.EnabledGrowthPacks, license.LastValidated, license.CreatedAt, license.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save camera license: %w", err)
	}

	return nil
}

// GetCamerasByTenant gets all camera licenses for a tenant
func (s *PostgresStorage) GetCamerasByTenant(ctx context.Context, tenantID string) ([]models.CameraLicense, error) {
	query := `
		SELECT id, camera_id, tenant_id, device_id, license_mode, is_valid, valid_until,
			   enabled_growth_packs, last_validated, created_at, updated_at
		FROM camera_licenses WHERE tenant_id = $1
	`

	rows, err := s.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get cameras: %w", err)
	}
	defer rows.Close()

	var cameras []models.CameraLicense
	for rows.Next() {
		var license models.CameraLicense
		err := rows.Scan(
			&license.ID, &license.CameraID, &license.TenantID, &license.DeviceID,
			&license.LicenseMode, &license.IsValid, &license.ValidUntil,
			&license.EnabledGrowthPacks, &license.LastValidated, &license.CreatedAt, &license.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan camera: %w", err)
		}
		cameras = append(cameras, license)
	}

	return cameras, nil
}

// CountCamerasByTenant counts cameras for a tenant
func (s *PostgresStorage) CountCamerasByTenant(ctx context.Context, tenantID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM camera_licenses WHERE tenant_id = $1", tenantID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count cameras: %w", err)
	}
	return count, nil
}

// =====================================
// Entitlement Operations
// =====================================

// GetEntitlement retrieves a feature entitlement
func (s *PostgresStorage) GetEntitlement(ctx context.Context, tenantID, category, feature string) (*models.FeatureEntitlement, error) {
	query := `
		SELECT id, tenant_id, feature_category, feature_name, is_enabled, quota_limit, quota_used, valid_until
		FROM feature_entitlements
		WHERE tenant_id = $1 AND feature_category = $2 AND feature_name = $3
	`

	var ent models.FeatureEntitlement
	err := s.pool.QueryRow(ctx, query, tenantID, category, feature).Scan(
		&ent.ID, &ent.TenantID, &ent.FeatureCategory, &ent.FeatureName,
		&ent.IsEnabled, &ent.QuotaLimit, &ent.QuotaUsed, &ent.ValidUntil,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get entitlement: %w", err)
	}

	return &ent, nil
}

// SaveEntitlement creates or updates a feature entitlement
func (s *PostgresStorage) SaveEntitlement(ctx context.Context, ent *models.FeatureEntitlement) error {
	query := `
		INSERT INTO feature_entitlements (id, tenant_id, feature_category, feature_name, is_enabled, quota_limit, quota_used, valid_until, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (tenant_id, feature_category, feature_name) DO UPDATE SET
			is_enabled = EXCLUDED.is_enabled, quota_limit = EXCLUDED.quota_limit, quota_used = EXCLUDED.quota_used,
			valid_until = EXCLUDED.valid_until, updated_at = EXCLUDED.updated_at
	`

	now := time.Now()
	_, err := s.pool.Exec(ctx, query,
		ent.ID, ent.TenantID, ent.FeatureCategory, ent.FeatureName,
		ent.IsEnabled, ent.QuotaLimit, ent.QuotaUsed, ent.ValidUntil, now, now,
	)
	if err != nil {
		return fmt.Errorf("failed to save entitlement: %w", err)
	}

	return nil
}

// =====================================
// Usage Event Operations
// =====================================

// SaveUsageEvents saves a batch of usage events
func (s *PostgresStorage) SaveUsageEvents(ctx context.Context, events []models.UsageEvent) error {
	if len(events) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	query := `
		INSERT INTO usage_events (tenant_id, event_type, resource_id, quantity, unit, metadata, event_time, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	for _, event := range events {
		batch.Queue(query, event.TenantID, event.EventType, event.ResourceID,
			event.Quantity, event.Unit, event.Metadata, event.EventTime, time.Now())
	}

	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()

	for i := 0; i < len(events); i++ {
		_, err := br.Exec()
		if err != nil {
			return fmt.Errorf("failed to save usage event %d: %w", i, err)
		}
	}

	return nil
}

// GetUsageSummary gets usage summary for a tenant within a time range
func (s *PostgresStorage) GetUsageSummary(ctx context.Context, tenantID string, start, end time.Time) (map[string]float64, error) {
	query := `
		SELECT event_type, SUM(quantity) as total
		FROM usage_events
		WHERE tenant_id = $1 AND event_time >= $2 AND event_time <= $3
		GROUP BY event_type
	`

	rows, err := s.pool.Query(ctx, query, tenantID, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to get usage summary: %w", err)
	}
	defer rows.Close()

	summary := make(map[string]float64)
	for rows.Next() {
		var eventType string
		var total float64
		if err := rows.Scan(&eventType, &total); err != nil {
			return nil, fmt.Errorf("failed to scan usage: %w", err)
		}
		summary[eventType] = total
	}

	return summary, nil
}

// =====================================
// Edge Device Operations
// =====================================

// SaveEdgeDevice creates or updates an edge device
func (s *PostgresStorage) SaveEdgeDevice(ctx context.Context, device *models.EdgeDevice) error {
	query := `
		INSERT INTO edge_devices (id, device_id, tenant_id, name, status, management_tier, last_heartbeat, active_camera_count, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (device_id) DO UPDATE SET
			status = EXCLUDED.status, last_heartbeat = EXCLUDED.last_heartbeat,
			active_camera_count = EXCLUDED.active_camera_count, updated_at = EXCLUDED.updated_at
	`

	now := time.Now()
	_, err := s.pool.Exec(ctx, query,
		device.ID, device.DeviceID, device.TenantID, device.Name, device.Status,
		device.ManagementTier, device.LastHeartbeat, device.ActiveCameraCount, now, now,
	)
	if err != nil {
		return fmt.Errorf("failed to save edge device: %w", err)
	}

	return nil
}

// GetEdgeDevice retrieves an edge device by device ID
func (s *PostgresStorage) GetEdgeDevice(ctx context.Context, deviceID string) (*models.EdgeDevice, error) {
	query := `
		SELECT id, device_id, tenant_id, name, status, management_tier, last_heartbeat, active_camera_count, created_at, updated_at
		FROM edge_devices WHERE device_id = $1
	`

	var device models.EdgeDevice
	err := s.pool.QueryRow(ctx, query, deviceID).Scan(
		&device.ID, &device.DeviceID, &device.TenantID, &device.Name, &device.Status,
		&device.ManagementTier, &device.LastHeartbeat, &device.ActiveCameraCount, &device.CreatedAt, &device.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get edge device: %w", err)
	}

	return &device, nil
}

// =====================================
// Statistics
// =====================================

// GetStats returns storage statistics
func (s *PostgresStorage) GetStats(ctx context.Context) (map[string]int, error) {
	stats := make(map[string]int)

	// Count tenants
	var tenantCount int
	s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM tenants").Scan(&tenantCount)
	stats["tenants"] = tenantCount

	// Count usage events
	var eventCount int
	s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM usage_events").Scan(&eventCount)
	stats["usage_events"] = eventCount

	// Count cameras
	var cameraCount int
	s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM camera_licenses").Scan(&cameraCount)
	stats["cameras"] = cameraCount

	// Count devices
	var deviceCount int
	s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM edge_devices").Scan(&deviceCount)
	stats["devices"] = deviceCount

	return stats, nil
}
