//go:build !windows && !darwin && !linux

package secrets

import "fmt"

// newStore factory: unsupported platforms return an error.
// Linux already has a standalone AES-GCM file implementation (secretstore_linux.go).
func newStore(dir string) (Store, error) {
	return nil, fmt.Errorf("当前平台不支持 SecretStore，仅支持 Windows、macOS 和 Linux")
}
