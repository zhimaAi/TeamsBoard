package capability

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
)

// GinContextGrantKey is the context key for local HTTP middleware write capability authorization.
const GinContextGrantKey = "goteams_capability_grant"

// Grant describes the local Skill and task-level resource range that a CLI Session can call.
type Grant struct {
	Skills          map[string]bool
	APICollectionID int64
	APIFolderID     int64
}

// Allows determines whether the authorization includes the specified Skill.
func (g Grant) Allows(skill string) bool {
	return g.Skills[skill]
}

// Registry only saves short-term capability tokens within the current client process.
type Registry struct {
	mu     sync.RWMutex
	grants map[string]Grant
}

// NewRegistry creates a capability token registry.
func NewRegistry() *Registry {
	return &Registry{grants: make(map[string]Grant)}
}

// Issue issues a random token. The token is only injected into the child process environment by the caller and is not persisted.
func (r *Registry) Issue(grant Grant) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	r.mu.Lock()
	r.grants[token] = cloneGrant(grant)
	r.mu.Unlock()
	return token, nil
}

// Lookup query token authorization.
func (r *Registry) Lookup(token string) (Grant, bool) {
	if r == nil || token == "" {
		return Grant{}, false
	}
	r.mu.RLock()
	grant, ok := r.grants[token]
	r.mu.RUnlock()
	if !ok {
		return Grant{}, false
	}
	return cloneGrant(grant), true
}

// Revoke revokes the token immediately.
func (r *Registry) Revoke(token string) {
	if r == nil || token == "" {
		return
	}
	r.mu.Lock()
	delete(r.grants, token)
	r.mu.Unlock()
}

func cloneGrant(grant Grant) Grant {
	cloned := grant
	cloned.Skills = make(map[string]bool, len(grant.Skills))
	for name, allowed := range grant.Skills {
		cloned.Skills[name] = allowed
	}
	return cloned
}
