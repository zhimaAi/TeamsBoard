package executor

import (
	"context"
	"sync"
	"time"
)

// cliDiscoveryRefreshInterval is how often the installed CLI list and the model
// lists are re-detected in the background. Detection is expensive (it spawns
// each CLI to read its version / model list), so the results are cached in
// memory and persisted to clis.json / models.json, refreshed periodically
// instead of on every request.
const cliDiscoveryRefreshInterval = 3 * time.Minute

// cliDiscoveryCache holds the latest discovery results (installed CLI list and
// per-CLI model lists) plus the on-disk persistence handle. Reads are cheap (a
// copy under RWMutex); writes happen when a refresh completes.
type cliDiscoveryCache struct {
	mu         sync.RWMutex
	items      []DiscoveredCLI
	models     map[string][]string
	lastSync   time.Time
	refreshing bool
	persist    *discoveryPersist
}

var defaultCLIDiscovery = &cliDiscoveryCache{
	models: map[string][]string{},
}

// refreshMu guards refreshCLIAndModels so periodic refreshes never overlap
// (a slow model probe must not pile up behind the ticker).
var refreshMu sync.Mutex

// modelProbeMu serializes live model probes triggered by HTTP requests so
// concurrent pickers do not spawn the same CLI repeatedly.
var modelProbeMu sync.Mutex

// StartCLIDiscoveryRefresh primes the discovery cache and keeps it fresh:
//
//  1. loads the on-disk snapshots (clis.json / models.json) into memory if
//     present;
//  2. re-detects the installed CLI list and the model list of every installed
//     CLI asynchronously, updating memory and persisting to disk;
//  3. repeats the detection every cliDiscoveryRefreshInterval until ctx is
//     cancelled.
//
// The background work never blocks HTTP handlers: they serve the on-disk
// snapshots directly when present (see GetCLIDiscovery / GetCLIModels).
func StartCLIDiscoveryRefresh(ctx context.Context, configDir string) {
	persist := newDiscoveryPersist(configDir)
	cache := defaultCLIDiscovery
	cache.mu.Lock()
	cache.persist = persist
	cache.mu.Unlock()

	go persist.runWriteQueue(ctx)

	go func() {
		// Seed memory from the last snapshot so the cache is never empty even
		// while the first full detection is still running.
		if items, ok := persist.loadCLIs(); ok {
			cache.setItems(items)
		}
		if models, ok := persist.loadModels(); ok {
			cache.setModels(models)
		}

		refreshCLIAndModels(context.Background())

		ticker := time.NewTicker(cliDiscoveryRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refreshCLIAndModels(context.Background())
			}
		}
	}()
}

// refreshCLIAndModels re-detects the installed CLI list and the model list of
// every installed CLI, updates the in-memory cache and persists the latest
// snapshots. Overlapping runs are skipped.
func refreshCLIAndModels(ctx context.Context) {
	if !refreshMu.TryLock() {
		return
	}
	defer refreshMu.Unlock()

	items := DiscoverCLIs(ctx)
	cache := defaultCLIDiscovery
	cache.setItems(items)

	models := make(map[string][]string, len(items))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, item := range items {
		if !item.Installed {
			continue
		}
		wg.Add(1)
		go func(cliType string) {
			defer wg.Done()
			probed := DiscoverCLIModels(ctx, cliType)
			mu.Lock()
			models[cliType] = probed
			mu.Unlock()
		}(item.Type)
	}
	wg.Wait()
	cache.setModels(models)

	cache.persistSnapshot()
}

// GetCLIDiscovery returns the installed CLI list. The in-memory cache is
// served first (it is always at least as fresh as the on-disk snapshot);
// when it has not been populated yet the on-disk snapshot (clis.json) is
// used so the picker opens without spawning any CLI. Only when neither is
// available is a synchronous detection performed and persisted.
func GetCLIDiscovery(ctx context.Context) []DiscoveredCLI {
	cache := defaultCLIDiscovery

	cache.mu.RLock()
	ready := !cache.lastSync.IsZero()
	if ready {
		items := cloneCLIList(cache.items)
		cache.mu.RUnlock()
		return items
	}
	cache.mu.RUnlock()

	if p := cache.getPersist(); p != nil {
		if items, ok := p.loadCLIs(); ok {
			if items == nil {
				return []DiscoveredCLI{}
			}
			return items
		}
	}

	_ = RefreshCLIDiscovery(ctx)
	cache.mu.RLock()
	items := cloneCLIList(cache.items)
	cache.mu.RUnlock()
	return items
}

// GetCLIModels returns the model list for a CLI type. The in-memory cache is
// served first; when it has no entry for the type the on-disk snapshot
// (models.json) is used. Only when both lack the type is the CLI spawned to
// probe live; live probes are serialized so concurrent requests do not spawn
// the same CLI repeatedly.
func GetCLIModels(ctx context.Context, cliType string) []string {
	cache := defaultCLIDiscovery

	if models, ok := cache.getModel(cliType); ok {
		return models
	}

	if p := cache.getPersist(); p != nil {
		if all, ok := p.loadModels(); ok {
			if models := all[cliType]; models != nil {
				return models
			}
		}
	}

	modelProbeMu.Lock()
	defer modelProbeMu.Unlock()

	// While waiting for the probe lock another request may have probed and
	// cached this CLI; re-check memory and the snapshot to avoid a duplicate
	// spawn.
	if models, ok := cache.getModel(cliType); ok {
		return models
	}
	if p := cache.getPersist(); p != nil {
		if all, ok := p.loadModels(); ok {
			if models := all[cliType]; models != nil {
				return models
			}
		}
	}

	models := DiscoverCLIModels(ctx, cliType)
	cache.setModel(cliType, models)
	cache.persistSnapshot()
	return models
}

// RefreshCLIDiscovery forces a synchronous re-detection of the CLI list and
// updates the cache. Concurrent callers share the same in-flight refresh to
// avoid spawning each CLI several times at once.
func RefreshCLIDiscovery(ctx context.Context) error {
	cache := defaultCLIDiscovery

	cache.mu.Lock()
	if cache.refreshing {
		cache.mu.Unlock()
		return nil
	}
	cache.refreshing = true
	cache.mu.Unlock()

	defer func() {
		cache.mu.Lock()
		cache.refreshing = false
		cache.mu.Unlock()
	}()

	items := DiscoverCLIs(ctx)

	cache.setItems(items)
	cache.persistSnapshot()
	return nil
}

// cloneCLIList returns a copy of the cached items so callers cannot mutate the
// cached state. The DiscoverCLIs output carries no model data, so a shallow
// copy of the outer slice is sufficient.
func cloneCLIList(items []DiscoveredCLI) []DiscoveredCLI {
	if items == nil {
		return []DiscoveredCLI{}
	}
	out := make([]DiscoveredCLI, len(items))
	copy(out, items)
	return out
}

func (c *cliDiscoveryCache) getPersist() *discoveryPersist {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.persist
}

func (c *cliDiscoveryCache) setItems(items []DiscoveredCLI) {
	c.mu.Lock()
	c.items = items
	c.lastSync = time.Now()
	c.mu.Unlock()
}

func (c *cliDiscoveryCache) setModels(models map[string][]string) {
	c.mu.Lock()
	c.models = models
	c.mu.Unlock()
}

func (c *cliDiscoveryCache) getModel(cliType string) ([]string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	models, ok := c.models[cliType]
	return models, ok
}

func (c *cliDiscoveryCache) setModel(cliType string, models []string) {
	c.mu.Lock()
	if c.models == nil {
		c.models = map[string][]string{}
	}
	c.models[cliType] = models
	c.mu.Unlock()
}

// persistSnapshot submits the latest in-memory snapshots to the write queue
// (non-blocking; the queue serializes actual disk writes).
func (c *cliDiscoveryCache) persistSnapshot() {
	p := c.getPersist()
	if p == nil {
		return
	}
	c.mu.RLock()
	items := cloneCLIList(c.items)
	models := make(map[string][]string, len(c.models))
	for k, v := range c.models {
		models[k] = append([]string(nil), v...)
	}
	c.mu.RUnlock()
	p.saveLatest(items, models)
}
