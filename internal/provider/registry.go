package provider

import (
	"fmt"
	"sort"
	"sync"
)

// Factory creates a Provider with the given base directory.
// If baseDir is empty, the provider uses its default directory.
type Factory func(baseDir string) Provider

var (
	mu       sync.RWMutex
	registry = make(map[string]Factory)
)

// Register adds a provider factory to the global registry.
// It panics if a provider with the same name is already registered.
// This is intended to be called from provider packages' init() functions.
func Register(name string, factory Factory) {
	mu.Lock()
	defer mu.Unlock()
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("provider: duplicate registration for %q", name))
	}
	registry[name] = factory
}

// New creates a provider by name with an optional base directory override.
// If baseDir is empty, the provider uses its default directory.
func New(name, baseDir string) (Provider, error) {
	mu.RLock()
	defer mu.RUnlock()
	factory, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("provider: unknown provider %q", name)
	}
	return factory(baseDir), nil
}

// NewWithDisplayName creates a provider by name with a custom display name.
// Useful for multiple directory targets that share the "directory" provider
// but need distinct names in output.
func NewWithDisplayName(providerName, baseDir, displayName string) (Provider, error) {
	p, err := New(providerName, baseDir)
	if err != nil {
		return nil, err
	}
	// If the underlying provider supports renaming, apply the display name.
	if smd, ok := p.(*skillMDProvider); ok {
		smd.providerName = displayName
	}
	return p, nil
}

// Get returns a provider with its default base directory.
func Get(name string) (Provider, error) {
	return New(name, "")
}

// IsRegistered reports whether a provider with the given name is registered.
func IsRegistered(name string) bool {
	mu.RLock()
	defer mu.RUnlock()
	_, ok := registry[name]
	return ok
}

// List returns the names of all registered providers, sorted alphabetically.
func List() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// resetRegistry clears all registered providers. Used only in tests.
func resetRegistry() {
	mu.Lock()
	defer mu.Unlock()
	registry = make(map[string]Factory)
}
