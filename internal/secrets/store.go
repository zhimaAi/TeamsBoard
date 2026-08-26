package secrets

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// KeyPrefix is the common prefix for secret reference names
const KeyPrefix = "goteams:"

// Predefined secret reference names
const (
	KeyJWT              = KeyPrefix + "jwt"
	KeyDeviceCredential = KeyPrefix + "device_credential"
	KeyLastLogin        = KeyPrefix + "last_login"
	// KeyLastLoginCustom 是 custom 登录索引的固定 key：custom 地址由用户主动选择、
	// 不受内置配置约束，无法用按地址派生的 key 定位，因此额外写入一份固定索引供恢复。
	KeyLastLoginCustom = KeyPrefix + "last_login_custom"
)

// ErrNotFound means the secret does not exist
var ErrNotFound = errors.New("密钥不存在")

// Store is the secret storage interface
type Store interface {
	// Set stores a secret; key is the reference name and value is the plaintext secret
	Set(key, value string) error
	// Get reads a secret
	Get(key string) (string, error)
	// Delete removes a secret
	Delete(key string) error
	// Exists checks whether a secret exists
	Exists(key string) (bool, error)
}

// New creates the platform-specific secret store implementation under the given directory.
// On Windows it is a DPAPI-encrypted file <dir>/secrets.json; on macOS it is Keychain (dir ignored);
// on Linux it is an AES-GCM-encrypted file <dir>/secrets.json (key derived from machine identity).
func New(configDir string) (Store, error) {
	return newStore(configDir)
}

// DelegatingStore forwards all calls to the "current backend" Store.
// Used to switch the secret store to an account-specific directory at login without reconstructing all objects holding a Store.
type DelegatingStore struct {
	mu      sync.RWMutex
	current Store
}

// NewDelegating creates a backend-switchable delegating store.
func NewDelegating() *DelegatingStore {
	return &DelegatingStore{}
}

// SetBackend switches the currently active account-specific backend store.
func (d *DelegatingStore) SetBackend(s Store) {
	d.mu.Lock()
	d.current = s
	d.mu.Unlock()
}

func (d *DelegatingStore) backend() Store {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.current
}

// Set writes a secret
func (d *DelegatingStore) Set(key, value string) error {
	b := d.backend()
	if b == nil {
		return fmt.Errorf("SecretStore 未初始化")
	}
	return b.Set(key, value)
}

// Get reads a secret
func (d *DelegatingStore) Get(key string) (string, error) {
	b := d.backend()
	if b == nil {
		return "", fmt.Errorf("SecretStore 未初始化")
	}
	return b.Get(key)
}

// Delete removes a secret
func (d *DelegatingStore) Delete(key string) error {
	b := d.backend()
	if b == nil {
		return fmt.Errorf("SecretStore 未初始化")
	}
	return b.Delete(key)
}

// Exists checks whether a secret exists
func (d *DelegatingStore) Exists(key string) (bool, error) {
	b := d.backend()
	if b == nil {
		return false, fmt.Errorf("SecretStore 未初始化")
	}
	return b.Exists(key)
}

// withPrefix ensures the secret reference name carries the goteams: prefix
func withPrefix(key string) string {
	if strings.HasPrefix(key, KeyPrefix) {
		return key
	}
	return KeyPrefix + key
}
