//go:build windows

package secrets

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"
)

var (
	modCrypt32    = syscall.NewLazyDLL("crypt32.dll")
	procProtect   = modCrypt32.NewProc("CryptProtectData")
	procUnprotect = modCrypt32.NewProc("CryptUnprotectData")
	modKernel32   = syscall.NewLazyDLL("kernel32.dll")
	procLocalFree = modKernel32.NewProc("LocalFree")
)

// dataBlob corresponds to the Windows DATA_BLOB structure
type dataBlob struct {
	cbData uint32
	pbData *byte
}

// dpapiStore is a secret store backed by Windows DPAPI
type dpapiStore struct {
	mu       sync.Mutex
	filePath string
}

// newDPAPIStore creates a DPAPI secret store under the given config directory
func newDPAPIStore(configDir string) (*dpapiStore, error) {
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("创建配置目录失败: %w", err)
	}
	return &dpapiStore{
		filePath: filepath.Join(configDir, "secrets.json"),
	}, nil
}

// newStore factory: on Windows returns the DPAPI implementation
func newStore(dir string) (Store, error) {
	return newDPAPIStore(dir)
}

// protectData encrypts data using DPAPI CryptProtectData
func protectData(plaintext []byte) ([]byte, error) {
	var inBlob dataBlob
	inBlob.cbData = uint32(len(plaintext))
	if len(plaintext) > 0 {
		inBlob.pbData = &plaintext[0]
	}

	var outBlob dataBlob

	ret, _, err := procProtect.Call(
		uintptr(unsafe.Pointer(&inBlob)),
		0, // szDataDescr
		0, // pOptionalEntropy
		0, // pvReserved
		0, // pPromptStruct
		0, // dwFlags
		uintptr(unsafe.Pointer(&outBlob)),
	)
	if ret == 0 {
		return nil, fmt.Errorf("CryptProtectData 失败: %w", err)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(outBlob.pbData)))

	out := make([]byte, outBlob.cbData)
	copy(out, unsafe.Slice(outBlob.pbData, outBlob.cbData))
	return out, nil
}

// unprotectData decrypts data using DPAPI CryptUnprotectData
func unprotectData(ciphertext []byte) ([]byte, error) {
	var inBlob dataBlob
	inBlob.cbData = uint32(len(ciphertext))
	if len(ciphertext) > 0 {
		inBlob.pbData = &ciphertext[0]
	}

	var outBlob dataBlob

	ret, _, err := procUnprotect.Call(
		uintptr(unsafe.Pointer(&inBlob)),
		0, // ppszDataDescr
		0, // pOptionalEntropy
		0, // pvReserved
		0, // pPromptStruct
		0, // dwFlags
		uintptr(unsafe.Pointer(&outBlob)),
	)
	if ret == 0 {
		return nil, fmt.Errorf("CryptUnprotectData 失败: %w", err)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(outBlob.pbData)))

	out := make([]byte, outBlob.cbData)
	copy(out, unsafe.Slice(outBlob.pbData, outBlob.cbData))
	return out, nil
}

// Set stores a secret
func (s *dpapiStore) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key = withPrefix(key)

	encrypted, err := protectData([]byte(value))
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

	data[key] = base64.StdEncoding.EncodeToString(encrypted)

	return s.save(data)
}

// Get reads a secret
func (s *dpapiStore) Get(key string) (string, error) {
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

	ciphertext, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("解码密钥失败: %w", err)
	}

	plaintext, err := unprotectData(ciphertext)
	if err != nil {
		return "", fmt.Errorf("解密密钥失败: %w", err)
	}

	return string(plaintext), nil
}

// Delete removes a secret
func (s *dpapiStore) Delete(key string) error {
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
func (s *dpapiStore) Exists(key string) (bool, error) {
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
func (s *dpapiStore) load() (map[string]string, error) {
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
func (s *dpapiStore) save(data map[string]string) error {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化密钥文件失败: %w", err)
	}
	return os.WriteFile(s.filePath, raw, 0600)
}
