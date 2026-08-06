//go:build linux

package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
)

// Linux-side key-derivation parameters.
const (
	linuxKeyLen  = 32 // AES-256
	linuxSaltLen = 16
	// linuxHKDFInfo is the HKDF info field, isolating this app's key-derivation context.
	linuxHKDFInfo = "goteams-secretstore-v1"
)

// linuxStore is a Linux secret store using AES-GCM with a key derived from the machine identity.
//
// Design trade-offs:
//   - Consistent interface and file layout with Windows (DPAPI file) and macOS (Keychain)
//     (<dir>/secrets.json with base64 ciphertext values) for easy per-account-directory isolation.
//   - No dependency on CGO system libraries like libsecret/keyring, ensuring CGO_ENABLED=0 cross-compilation works.
//   - Encryption key = scrypt(machine identity, salt): prefer /etc/machine-id for the machine identity,
//     falling back to hostname+username; salt is persisted at <dir>/secrets.salt (0600).
//     This way, secrets.json copied alone cannot be decrypted (requires the same-source machine identity), avoiding plaintext leakage.
type linuxStore struct {
	mu           sync.Mutex
	filePath     string // secrets.json
	saltFilePath string // secrets.salt
}

// newLinuxStore creates a Linux secret store under the specified config directory.
func newLinuxStore(dir string) (*linuxStore, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("创建配置目录失败: %w", err)
	}
	return &linuxStore{
		filePath:     filepath.Join(dir, "secrets.json"),
		saltFilePath: filepath.Join(dir, "secrets.salt"),
	}, nil
}

// newStore factory: on Linux returns the AES-GCM file implementation.
func newStore(dir string) (Store, error) {
	return newLinuxStore(dir)
}

// machineSecret obtains the machine identity used to derive the encryption key.
func machineSecret() (string, error) {
	// Most Linux distributions have /etc/machine-id, bound to the hardware/system installation.
	if raw, err := os.ReadFile("/etc/machine-id"); err == nil {
		s := strings.TrimSpace(string(raw))
		if s != "" {
			return s, nil
		}
	}
	// Fallback: hostname + username (cross-platform).
	host, _ := os.Hostname()
	name := ""
	if u, err := user.Current(); err == nil {
		name = u.Username
	}
	if host == "" && name == "" {
		return "", errors.New("无法获取机器标识用于派生密钥")
	}
	return host + "\x00" + name, nil
}

// loadOrCreateSalt reads or generates and persists the salt.
func (s *linuxStore) loadOrCreateSalt() ([]byte, error) {
	if raw, err := os.ReadFile(s.saltFilePath); err == nil {
		if len(raw) >= 8 {
			return raw, nil
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("读取 salt 失败: %w", err)
	}
	salt := make([]byte, linuxSaltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("生成 salt 失败: %w", err)
	}
	if err := os.WriteFile(s.saltFilePath, salt, 0600); err != nil {
		return nil, fmt.Errorf("写入 salt 失败: %w", err)
	}
	return salt, nil
}

// deriveKey derives the AES key using HKDF (standard library crypto/hkdf, Go 1.26 generic API)
// from machine identity + salt. HKDF is chosen over scrypt because it is in the standard library and pure Go cross-compilable,
// and the machine identity already has sufficient entropy. hkdf.Key does Extract+Expand in one call.
func (s *linuxStore) deriveKey() ([]byte, error) {
	secret, err := machineSecret()
	if err != nil {
		return nil, err
	}
	salt, err := s.loadOrCreateSalt()
	if err != nil {
		return nil, err
	}
	key, err := hkdf.Key(sha256.New, []byte(secret), salt, linuxHKDFInfo, linuxKeyLen)
	if err != nil {
		return nil, fmt.Errorf("派生密钥失败: %w", err)
	}
	return key, nil
}

// encrypt encrypts plaintext and returns base64(nonce || ciphertext).
func (s *linuxStore) encrypt(plaintext []byte) (string, error) {
	key, err := s.deriveKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("创建密码块失败: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("创建 GCM 失败: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("生成随机数失败: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// decrypt decrypts base64(nonce || ciphertext).
func (s *linuxStore) decrypt(b64 string) ([]byte, error) {
	key, err := s.deriveKey()
	if err != nil {
		return nil, err
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("解码密钥失败: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("创建密码块失败: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("创建 GCM 失败: %w", err)
	}
	ns := gcm.NonceSize()
	if len(raw) < ns {
		return nil, errors.New("密钥数据损坏")
	}
	nonce, ciphertext := raw[:ns], raw[ns:]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("解密密钥失败: %w", err)
	}
	return plain, nil
}

// Set stores a secret
func (s *linuxStore) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key = withPrefix(key)

	enc, err := s.encrypt([]byte(value))
	if err != nil {
		return fmt.Errorf("加密密钥失败: %w", err)
	}

	data, err := s.load()
	if err != nil {
		return fmt.Errorf("读取密钥文件失败: %w", err)
	}
	if data == nil {
		data = make(map[string]string)
	}
	data[key] = enc

	return s.save(data)
}

// Get reads a secret
func (s *linuxStore) Get(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key = withPrefix(key)

	data, err := s.load()
	if err != nil {
		return "", fmt.Errorf("读取密钥文件失败: %w", err)
	}
	b64, ok := data[key]
	if !ok {
		return "", ErrNotFound
	}

	plain, err := s.decrypt(b64)
	if err != nil {
		return "", fmt.Errorf("解密密钥失败: %w", err)
	}
	return string(plain), nil
}

// Delete removes a secret
func (s *linuxStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key = withPrefix(key)

	data, err := s.load()
	if err != nil {
		return fmt.Errorf("读取密钥文件失败: %w", err)
	}
	if data == nil {
		return nil // File does not exist, so the secret does not exist either
	}
	delete(data, key)

	return s.save(data)
}

// Exists checks whether a secret exists
func (s *linuxStore) Exists(key string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key = withPrefix(key)

	data, err := s.load()
	if err != nil {
		return false, fmt.Errorf("读取密钥文件失败: %w", err)
	}
	_, ok := data[key]
	return ok, nil
}

// load reads the secret file
func (s *linuxStore) load() (map[string]string, error) {
	raw, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var data map[string]string
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("解析密钥文件失败: %w", err)
	}
	return data, nil
}

// save writes the secret file
func (s *linuxStore) save(data map[string]string) error {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化密钥文件失败: %w", err)
	}
	return os.WriteFile(s.filePath, raw, 0600)
}
