// Package store provides thread-safe in-memory storage for Crossplane claim and XR metadata.
package store

import (
	"context"
	"sync"
	"time"
)

// ClaimInfo holds extracted metadata for a single Crossplane claim.
type ClaimInfo struct {
	GVR         string    `json:"gvr"` // "group/version/resource" used to track which GVR produced this entry
	Group       string    `json:"group"`
	Version     string    `json:"version"` // API version from the GVR
	Kind        string    `json:"kind"`
	Namespace   string    `json:"namespace"`
	Name        string    `json:"name"`
	Creator     string    `json:"creator"`     // from annotation, or empty
	Team        string    `json:"team"`        // from annotation, or empty
	Composition string    `json:"composition"` // resolved from XR reference/labels, or empty
	Paused      bool      `json:"paused"`      // crossplane.io/paused annotation
	Synced      bool      `json:"synced"`
	Ready       bool      `json:"ready"`
	Reason      string    `json:"reason"`              // Ready condition reason
	CreatedAt   time.Time `json:"createdAt"`           // metadata.creationTimestamp
	DeletedAt   time.Time `json:"deletedAt,omitempty"` // metadata.deletionTimestamp, zero when not deleting
	XRRef       string    `json:"xrRef"`               // spec.resourceRef.name — used for composition enrichment
}

// XRInfo holds extracted metadata for a single Crossplane composite resource.
type XRInfo struct {
	GVR         string    `json:"gvr"` // "group/version/resource"
	Group       string    `json:"group"`
	Version     string    `json:"version"` // API version from the GVR
	Kind        string    `json:"kind"`
	Namespace   string    `json:"namespace"` // usually empty for cluster-scoped XRs
	Name        string    `json:"name"`
	ClaimName   string    `json:"claimName"`
	ClaimNS     string    `json:"claimNamespace"`
	Composition string    `json:"composition"`
	Paused      bool      `json:"paused"` // crossplane.io/paused annotation
	Synced      bool      `json:"synced"`
	Ready       bool      `json:"ready"`
	Reason      string    `json:"reason"`              // Ready condition reason
	CreatedAt   time.Time `json:"createdAt"`           // metadata.creationTimestamp
	DeletedAt   time.Time `json:"deletedAt,omitempty"` // metadata.deletionTimestamp, zero when not deleting
}

// MRInfo holds extracted metadata for a single Crossplane provider Managed Resource.
type MRInfo struct {
	GVR                string    `json:"gvr"`
	Group              string    `json:"group"`
	Version            string    `json:"version"` // API version from the GVR
	Kind               string    `json:"kind"`
	Namespace          string    `json:"namespace"`
	Name               string    `json:"name"`
	XRName             string    `json:"xrName"`             // crossplane.io/composite label
	ClaimName          string    `json:"claimName"`          // enriched from XR or MR labels
	ClaimNS            string    `json:"claimNamespace"`     // enriched from XR or MR labels
	Provider           string    `json:"provider"`           // pkg.crossplane.io/package from MRD discovery
	ProviderConfig     string    `json:"providerConfig"`     // spec.providerConfigRef.name
	ExternalName       string    `json:"externalName"`       // crossplane.io/external-name annotation
	ManagementPolicies string    `json:"managementPolicies"` // joined spec.managementPolicies
	Paused             bool      `json:"paused"`             // crossplane.io/paused annotation
	Synced             bool      `json:"synced"`
	Ready              bool      `json:"ready"`
	Reason             string    `json:"reason"`
	CreatedAt          time.Time `json:"createdAt"`
	DeletedAt          time.Time `json:"deletedAt,omitempty"` // metadata.deletionTimestamp, zero when not deleting
}

// Store is the interface for claim and XR metadata storage.
// Implementations must be safe for concurrent use.
type Store interface {
	ReplaceClaims(gvr string, items []ClaimInfo)
	ReplaceXRs(gvr string, items []XRInfo)
	ReplaceMRs(gvr string, items []MRInfo)
	EnrichClaimCompositions()
	EnrichXRClaims()
	EnrichMRClaims()
	SnapshotClaims() []ClaimInfo
	SnapshotXRs() []XRInfo
	SnapshotMRs() []MRInfo
	ClaimCount() int
	XRCount() int
	MRCount() int
}

// PersistentStore extends Store with durable persistence capabilities.
// Implementations wrap a MemoryStore, delegate all Store methods to it,
// and add persistence after each poll cycle plus one-time restore on startup.
type PersistentStore interface {
	Store
	Persist(ctx context.Context) error
	Restore(ctx context.Context) error
}

// Snapshot is the serialisation envelope for persisting store state.
// All PersistentStore implementations should use this struct to ensure
// a consistent format across backends.
type Snapshot struct {
	Claims      []ClaimInfo `json:"claims"`
	XRs         []XRInfo    `json:"xrs"`
	MRs         []MRInfo    `json:"mrs,omitempty"`
	PersistedAt time.Time   `json:"persistedAt"`
}

// MemoryStore is a thread-safe in-memory implementation of Store.
// All public methods are safe for concurrent use.
type MemoryStore struct {
	mu     sync.RWMutex
	claims map[string]ClaimInfo // keyed by "namespace/name"
	xrs    map[string]XRInfo    // keyed by "namespace/name" (or just "name" for cluster-scoped)
	mrs    map[string]MRInfo    // keyed by "namespace/name"
}

// New creates a new empty MemoryStore.
func New() *MemoryStore {
	return &MemoryStore{
		claims: make(map[string]ClaimInfo),
		xrs:    make(map[string]XRInfo),
		mrs:    make(map[string]MRInfo),
	}
}

// ReplaceClaims atomically replaces the stored claims for a given GVR.
// The gvr string identifies which GVR produced these items (e.g. "group/version/resource").
// Items belonging to this GVR that are no longer present are removed.
// Items from other GVRs are left untouched.
func (s *MemoryStore) ReplaceClaims(gvr string, items []ClaimInfo) {
	newKeys := make(map[string]struct{}, len(items))
	s.mu.Lock()
	defer s.mu.Unlock()

	// Add/update incoming items.
	for _, c := range items {
		key := objectKey(c.Namespace, c.Name)
		newKeys[key] = struct{}{}
		s.claims[key] = c
	}

	// Remove stale entries belonging to this GVR.
	for key, existing := range s.claims {
		if existing.GVR != gvr {
			continue
		}
		if _, ok := newKeys[key]; !ok {
			delete(s.claims, key)
		}
	}
}

// ReplaceXRs atomically replaces the stored XRs for a given GVR.
func (s *MemoryStore) ReplaceXRs(gvr string, items []XRInfo) {
	newKeys := make(map[string]struct{}, len(items))
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, x := range items {
		key := objectKey(x.Namespace, x.Name)
		newKeys[key] = struct{}{}
		s.xrs[key] = x
	}

	for key, existing := range s.xrs {
		if existing.GVR != gvr {
			continue
		}
		if _, ok := newKeys[key]; !ok {
			delete(s.xrs, key)
		}
	}
}

// ReplaceMRs atomically replaces the stored MRs for a given GVR.
func (s *MemoryStore) ReplaceMRs(gvr string, items []MRInfo) {
	newKeys := make(map[string]struct{}, len(items))
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, m := range items {
		key := objectKey(m.Namespace, m.Name)
		newKeys[key] = struct{}{}
		s.mrs[key] = m
	}

	for key, existing := range s.mrs {
		if existing.GVR != gvr {
			continue
		}
		if _, ok := newKeys[key]; !ok {
			delete(s.mrs, key)
		}
	}
}

// EnrichXRClaims looks up each XR without claim labels in the claim store and
// copies ClaimName and ClaimNS from the claim whose spec.resourceRef.name
// matches the XR name. Label-derived values are not overwritten. Must be
// called after both claims and XRs have been replaced for the current polling
// cycle. If multiple claims reference the same XR, the first match wins.
func (s *MemoryStore) EnrichXRClaims() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, xr := range s.xrs {
		if xr.ClaimName != "" {
			continue
		}
		for _, claim := range s.claims {
			if claim.XRRef == xr.Name {
				xr.ClaimName = claim.Name
				xr.ClaimNS = claim.Namespace
				s.xrs[key] = xr
				break
			}
		}
	}
}

// EnrichClaimCompositions looks up each claim's XRRef in the XR store and
// copies the Composition value. Must be called after both claims and XRs
// have been replaced for the current polling cycle.
func (s *MemoryStore) EnrichClaimCompositions() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, claim := range s.claims {
		if claim.XRRef == "" {
			continue
		}
		// XRs are cluster-scoped, so look up by name only.
		if xr, ok := s.xrs[claim.XRRef]; ok {
			claim.Composition = xr.Composition
			s.claims[key] = claim
		}
	}
}

// EnrichMRClaims copies claim linkage onto MRs from the backing XR store when
// claim fields are not already set from MR labels. Must be called after
// claims, XRs, and MRs have been replaced for the current polling cycle.
func (s *MemoryStore) EnrichMRClaims() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, mr := range s.mrs {
		if mr.ClaimName != "" {
			continue
		}
		if mr.XRName == "" {
			continue
		}
		if xr, ok := s.xrs[mr.XRName]; ok {
			mr.ClaimName = xr.ClaimName
			mr.ClaimNS = xr.ClaimNS
			s.mrs[key] = mr
		}
	}
}

// SnapshotClaims returns a copy of all stored claims.
func (s *MemoryStore) SnapshotClaims() []ClaimInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ClaimInfo, 0, len(s.claims))
	for _, c := range s.claims {
		out = append(out, c)
	}
	return out
}

// SnapshotXRs returns a copy of all stored XRs.
func (s *MemoryStore) SnapshotXRs() []XRInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]XRInfo, 0, len(s.xrs))
	for _, x := range s.xrs {
		out = append(out, x)
	}
	return out
}

// SnapshotMRs returns a copy of all stored MRs.
func (s *MemoryStore) SnapshotMRs() []MRInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]MRInfo, 0, len(s.mrs))
	for _, m := range s.mrs {
		out = append(out, m)
	}
	return out
}

// ClaimCount returns the total number of stored claims.
func (s *MemoryStore) ClaimCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.claims)
}

// XRCount returns the total number of stored XRs.
func (s *MemoryStore) XRCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.xrs)
}

// MRCount returns the total number of stored MRs.
func (s *MemoryStore) MRCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.mrs)
}

// objectKey produces a map key from a namespace and name.
// For cluster-scoped resources (empty namespace) the key is just the name.
// For namespaced resources the key is "namespace/name".
func objectKey(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "/" + name
}
