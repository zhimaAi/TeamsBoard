package identity

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"goteams-client/internal/secrets"
)

// DeviceManager device identity management
type DeviceManager struct {
	configDir   string
	db          *sql.DB
	secretStore secrets.Store
}

// NewDeviceManager creates a device manager
func NewDeviceManager(configDir string, db *sql.DB, store secrets.Store) *DeviceManager {
	return &DeviceManager{
		configDir:   configDir,
		db:          db,
		secretStore: store,
	}
}

// GetOrCreateUUID gets or generates device UUID
func (m *DeviceManager) GetOrCreateUUID() (string, error) {
	uuidPath := filepath.Join(m.configDir, "device_uuid")

	//Try to read an existing UUID
	data, err := os.ReadFile(uuidPath)
	if err == nil && len(data) > 0 {
		uuidStr := strings.TrimSpace(string(data))
		if uuidStr != "" {
			if m.secretStore != nil {
				_ = m.secretStore.Set(secrets.KeyPrefix+"device_uuid", uuidStr)
			}
			return uuidStr, nil
		}
	}

	// Generate new UUID
	deviceUUID := uuid.New().String()
	if err := os.WriteFile(uuidPath, []byte(deviceUUID), 0600); err != nil {
		return "", fmt.Errorf("持久化设备 UUID 失败: %w", err)
	}

	// At the same time, store it in SecretStore for cloud client to read.
	if m.secretStore != nil {
		_ = m.secretStore.Set(secrets.KeyPrefix+"device_uuid", deviceUUID)
	}

	return deviceUUID, nil
}

// IsRegistered checks whether the device has been registered
func (m *DeviceManager) IsRegistered() bool {
	if m.db == nil {
		return false
	}
	var count int
	err := m.db.QueryRow("SELECT COUNT(*) FROM gt_device_registrations LIMIT 1").Scan(&count)
	if err != nil {
		return false
	}
	return count > 0
}

// SaveRegistration saves device registration information
func (m *DeviceManager) SaveRegistration(adminID, deviceUUID, credential string) error {
	// Save credentials to SecretStore
	if m.secretStore != nil && credential != "" {
		if err := m.secretStore.Set(secrets.KeyDeviceCredential, credential); err != nil {
			return fmt.Errorf("保存设备凭据失败: %w", err)
		}
	}

	//Write database records
	credentialRef := secrets.KeyDeviceCredential
	now := timeNow()
	_, err := m.db.Exec(
		`INSERT OR REPLACE INTO gt_device_registrations (admin_id, device_uuid, credential_ref, registered_at) VALUES (?, ?, ?, ?)`,
		adminID, deviceUUID, credentialRef, now)
	if err != nil {
		return fmt.Errorf("保存设备注册记录失败: %w", err)
	}

	return nil
}

// timeNow returns the current Unix millisecond timestamp
func timeNow() int64 {
	return timeNowMillis()
}
