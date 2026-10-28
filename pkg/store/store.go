// Package store provides thread-safe in-memory storage for Crossplane resource metadata.
package store

import (
	"context"
	"strings"
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
	GVR                 string        `json:"gvr"` // "group/version/resource"
	Group               string        `json:"group"`
	Version             string        `json:"version"` // API version from the GVR
	Kind                string        `json:"kind"`
	Namespace           string        `json:"namespace"` // usually empty for cluster-scoped XRs
	Name                string        `json:"name"`
	UID                 string        `json:"uid,omitempty"` // metadata.uid — used to validate ownerReferences
	ClaimName           string        `json:"claimName"`
	ClaimNS             string        `json:"claimNamespace"`
	Composition         string        `json:"composition"`
	ParentKind          string        `json:"parentKind,omitempty"`      // immediate XR owner kind from ownerReferences
	ParentName          string        `json:"parentName,omitempty"`      // immediate XR owner name; empty for roots
	ParentNamespace     string        `json:"parentNamespace,omitempty"` // resolved namespaced parent; empty for cluster-scoped / roots
	ParentGroup         string        `json:"parentGroup,omitempty"`     // API group of parent (internal join / validation)
	ParentUID           string        `json:"parentUID,omitempty"`       // ownerReference UID — required for parent match
	ResourceRefs        []ComposedRef `json:"resourceRefs,omitempty"`    // composed children from resourceRefs
	ClaimsUnsupported   bool          `json:"-"`                         // true for modern XRs that cannot be backed by claims
	Paused              bool          `json:"paused"`                    // crossplane.io/paused annotation
	Synced              bool          `json:"synced"`
	Ready               bool          `json:"ready"`
	Reason              string        `json:"reason"`                        // Ready condition reason
	ReadyMessage        string        `json:"readyMessage,omitempty"`        // Ready condition message (may be truncated at export)
	ReadyLastTransition time.Time     `json:"readyLastTransition,omitempty"` // Ready condition lastTransitionTime
	CompositionRevision string        `json:"compositionRevision,omitempty"` // compositionRevisionRef / label
	CreatedAt           time.Time     `json:"createdAt"`                     // metadata.creationTimestamp
	DeletedAt           time.Time     `json:"deletedAt,omitempty"`           // metadata.deletionTimestamp, zero when not deleting
}

// ComposedRef is a composed child listed on an XR (resourceRefs).
// Used for MR owner fallback and optional edge metrics — not a Prom label dump.
type ComposedRef struct {
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Name       string `json:"name,omitempty"`
}

// MRInfo holds extracted metadata for a single Crossplane provider Managed Resource.
type MRInfo struct {
	GVR                 string    `json:"gvr"`
	Group               string    `json:"group"`
	Version             string    `json:"version"` // API version from the GVR
	Kind                string    `json:"kind"`
	Namespace           string    `json:"namespace"`
	Name                string    `json:"name"`
	UID                 string    `json:"uid,omitempty"`              // metadata.uid
	XRName              string    `json:"xrName"`                     // crossplane.io/composite label
	XRKind              string    `json:"xrKind"`                     // enriched from backing XR kind
	XRNamespace         string    `json:"xrNamespace"`                // enriched from backing XR namespace
	XRGroup             string    `json:"xrGroup"`                    // enriched from backing XR API group
	OwnerXRName         string    `json:"ownerXRName,omitempty"`      // immediate composing XR name (ownerRefs / resourceRefs)
	OwnerXRKind         string    `json:"ownerXRKind,omitempty"`      // immediate composing XR kind
	OwnerXRNamespace    string    `json:"ownerXRNamespace,omitempty"` // immediate composing XR namespace
	OwnerXRGroup        string    `json:"ownerXRGroup,omitempty"`     // immediate composing XR API group
	OwnerXRUID          string    `json:"ownerXRUID,omitempty"`       // ownerReference UID — required for ownerRefs match
	ClaimName           string    `json:"claimName"`                  // enriched from XR or MR labels (legacy claim path)
	ClaimNS             string    `json:"claimNamespace"`             // enriched from XR or MR labels (legacy claim path)
	Provider            string    `json:"provider"`                   // pkg.crossplane.io/package from MRD discovery
	ProviderConfig      string    `json:"providerConfig"`             // spec.providerConfigRef.name
	ExternalName        string    `json:"externalName"`               // crossplane.io/external-name annotation
	ManagementPolicies  string    `json:"managementPolicies"`         // joined spec.managementPolicies
	Paused              bool      `json:"paused"`                     // crossplane.io/paused annotation
	Synced              bool      `json:"synced"`
	Ready               bool      `json:"ready"`
	Reason              string    `json:"reason"`
	ReadyMessage        string    `json:"readyMessage,omitempty"`        // Ready condition message
	ReadyLastTransition time.Time `json:"readyLastTransition,omitempty"` // Ready condition lastTransitionTime
	CreatedAt           time.Time `json:"createdAt"`
	DeletedAt           time.Time `json:"deletedAt,omitempty"` // metadata.deletionTimestamp, zero when not deleting
}

// PackageInfo holds extracted metadata for a single Crossplane package
// meta-type (Provider, Configuration, or Function). Package revisions are
// intentionally excluded — they are high-cardinality and out of scope.
type PackageInfo struct {
	GVR       string `json:"gvr"`
	Group     string `json:"group"`
	Version   string `json:"version"`
	Kind      string `json:"kind"` // Provider, Configuration, or Function
	Name      string `json:"name"`
	Healthy   bool   `json:"healthy"`   // Healthy condition True
	Installed bool   `json:"installed"` // Installed condition True
}

// Store is the interface for Crossplane resource metadata storage.
// Implementations must be safe for concurrent use.
type Store interface {
	ReplaceClaims(gvr string, items []ClaimInfo)
	ReplaceXRs(gvr string, items []XRInfo)
	ReplaceMRs(gvr string, items []MRInfo)
	ReplacePackages(gvr string, items []PackageInfo)
	EnrichClaimCompositions()
	EnrichXRClaims()
	EnrichXRParents()
	EnrichMRClaims()
	EnrichMROwners()
	SnapshotClaims() []ClaimInfo
	SnapshotXRs() []XRInfo
	SnapshotMRs() []MRInfo
	SnapshotPackages() []PackageInfo
	ClaimCount() int
	XRCount() int
	MRCount() int
	PackageCount() int
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
	Claims      []ClaimInfo   `json:"claims"`
	XRs         []XRInfo      `json:"xrs"`
	MRs         []MRInfo      `json:"mrs,omitempty"`
	Packages    []PackageInfo `json:"packages,omitempty"`
	PersistedAt time.Time     `json:"persistedAt"`
}

// MemoryStore is a thread-safe in-memory implementation of Store.
// All public methods are safe for concurrent use.
type MemoryStore struct {
	mu       sync.RWMutex
	claims   map[string]ClaimInfo   // keyed by "namespace/name"
	xrs      map[string]XRInfo      // keyed by "namespace/name", or "name" when cluster-scoped
	mrs      map[string]MRInfo      // keyed by "namespace/name"
	packages map[string]PackageInfo // keyed by kind/name (cluster-scoped)
}

// New creates a new empty MemoryStore.
func New() *MemoryStore {
	return &MemoryStore{
		claims:   make(map[string]ClaimInfo),
		xrs:      make(map[string]XRInfo),
		mrs:      make(map[string]MRInfo),
		packages: make(map[string]PackageInfo),
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
// Keys include the GVR so two XR kinds that share a namespace/name do not collide.
func (s *MemoryStore) ReplaceXRs(gvr string, items []XRInfo) {
	newKeys := make(map[string]struct{}, len(items))
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, x := range items {
		key := xrKey(gvr, x.Namespace, x.Name)
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

// ReplacePackages atomically replaces the stored packages for a given GVR.
// Keys are kind/name so Provider and Function objects with the same metadata
// name do not collide.
func (s *MemoryStore) ReplacePackages(gvr string, items []PackageInfo) {
	newKeys := make(map[string]struct{}, len(items))
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, p := range items {
		key := packageKey(p.Kind, p.Name)
		newKeys[key] = struct{}{}
		s.packages[key] = p
	}

	for key, existing := range s.packages {
		if existing.GVR != gvr {
			continue
		}
		if _, ok := newKeys[key]; !ok {
			delete(s.packages, key)
		}
	}
}

// EnrichXRClaims looks up each legacy XR without claim labels in the claim
// store and copies ClaimName and ClaimNS from the claim whose
// spec.resourceRef.name matches. Namespaced XRs only match claims in their own
// namespace; cluster-scoped XRs retain the legacy cross-namespace behaviour.
// Label-derived values are not overwritten.
func (s *MemoryStore) EnrichXRClaims() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, xr := range s.xrs {
		if xr.ClaimsUnsupported || xr.ClaimName != "" {
			continue
		}
		for _, claim := range s.claims {
			if claim.XRRef == xr.Name && (xr.Namespace == "" || xr.Namespace == claim.Namespace) {
				xr.ClaimName = claim.Name
				xr.ClaimNS = claim.Namespace
				s.xrs[key] = xr
				break
			}
		}
	}
}

// EnrichClaimCompositions looks up each claim's XRRef in the XR store and
// copies the Composition value. It prefers an XR in the claim's namespace,
// then falls back to a cluster-scoped XR for legacy claims.
func (s *MemoryStore) EnrichClaimCompositions() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, claim := range s.claims {
		if claim.XRRef == "" {
			continue
		}
		if xr, ok := s.lookupXR(claim.Namespace, claim.XRRef); ok {
			claim.Composition = xr.Composition
			s.claims[key] = claim
		}
	}
}

// EnrichXRParents validates convert-time parent_* candidates against the XR
// inventory. A parent must uniquely match an inventoried XR by kind+name
// (and group when set) and by ownerReference UID. ParentNamespace is filled
// from the matched XR (empty for cluster-scoped parents). Missing, UID-mismatched,
// or ambiguous parents are cleared so root / unknown linkage stays empty rather
// than guessing a same-name replacement.
func (s *MemoryStore) EnrichXRParents() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, xr := range s.xrs {
		if xr.ParentName == "" && xr.ParentKind == "" {
			continue
		}
		parent, ok := s.lookupParentXR(xr)
		if !ok {
			xr.ParentKind = ""
			xr.ParentName = ""
			xr.ParentNamespace = ""
			xr.ParentGroup = ""
			xr.ParentUID = ""
			s.xrs[key] = xr
			continue
		}
		xr.ParentKind = parent.Kind
		xr.ParentName = parent.Name
		xr.ParentNamespace = parent.Namespace
		xr.ParentGroup = parent.Group
		xr.ParentUID = parent.UID
		s.xrs[key] = xr
	}
}

// lookupParentXR finds the unique inventoried XR matching a child's parent_*
// candidate. Preference: same-namespace namespaced XR when the child is
// namespaced, then cluster-scoped XR with that name/kind. Requires a non-empty
// exact ParentUID ↔ UID match so orphaned children do not attach to a same-name
// replacement of the deleted owner.
func (s *MemoryStore) lookupParentXR(child XRInfo) (XRInfo, bool) {
	if child.ParentUID == "" {
		return XRInfo{}, false
	}
	var namespaced, cluster []XRInfo
	for _, cand := range s.xrs {
		if cand.Name != child.ParentName {
			continue
		}
		if child.ParentKind != "" && cand.Kind != child.ParentKind {
			continue
		}
		if child.ParentGroup != "" && cand.Group != child.ParentGroup {
			continue
		}
		if cand.UID == "" || cand.UID != child.ParentUID {
			continue
		}
		// Never treat the child as its own parent.
		if cand.UID == child.UID && child.UID != "" {
			continue
		}
		if cand.Name == child.Name && cand.Namespace == child.Namespace && cand.Kind == child.Kind && cand.Group == child.Group {
			continue
		}
		switch {
		case cand.Namespace == "":
			cluster = append(cluster, cand)
		case child.Namespace != "" && cand.Namespace == child.Namespace:
			namespaced = append(namespaced, cand)
		case child.ParentNamespace != "" && cand.Namespace == child.ParentNamespace:
			namespaced = append(namespaced, cand)
		}
	}
	if child.Namespace != "" && len(namespaced) == 1 {
		return namespaced[0], true
	}
	if child.Namespace != "" && len(namespaced) > 1 {
		return XRInfo{}, false
	}
	if len(cluster) == 1 {
		return cluster[0], true
	}
	return XRInfo{}, false
}

// EnrichMROwners resolves each MR's immediate composing XR.
// Preference: ownerReferences candidate from convert (validated against
// inventory); else a unique XR whose resourceRefs list this MR (GVK+name).
// Never copies xr_name into owner_xr_* (nesting must remain visible).
func (s *MemoryStore) EnrichMROwners() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, mr := range s.mrs {
		if owner, ok := s.resolveMROwner(mr); ok {
			mr.OwnerXRKind = owner.Kind
			mr.OwnerXRName = owner.Name
			mr.OwnerXRNamespace = owner.Namespace
			mr.OwnerXRGroup = owner.Group
			mr.OwnerXRUID = owner.UID
			s.mrs[key] = mr
			continue
		}
		if mr.OwnerXRName != "" || mr.OwnerXRKind != "" || mr.OwnerXRNamespace != "" || mr.OwnerXRGroup != "" || mr.OwnerXRUID != "" {
			mr.OwnerXRKind = ""
			mr.OwnerXRName = ""
			mr.OwnerXRNamespace = ""
			mr.OwnerXRGroup = ""
			mr.OwnerXRUID = ""
			s.mrs[key] = mr
		}
	}
}

func (s *MemoryStore) resolveMROwner(mr MRInfo) (XRInfo, bool) {
	if mr.OwnerXRName != "" {
		if owner, ok := s.lookupOwnerXR(mr); ok {
			return owner, true
		}
	}
	return s.findXRByResourceRef(mr)
}

// lookupOwnerXR finds the unique inventoried XR matching an MR ownerRefs
// candidate. Requires a non-empty exact OwnerXRUID ↔ UID match so orphaned
// MRs do not attach to a same-name replacement of the deleted owner.
func (s *MemoryStore) lookupOwnerXR(mr MRInfo) (XRInfo, bool) {
	if mr.OwnerXRUID == "" {
		return XRInfo{}, false
	}
	var namespaced, cluster []XRInfo
	for _, cand := range s.xrs {
		if cand.Name != mr.OwnerXRName {
			continue
		}
		if mr.OwnerXRKind != "" && cand.Kind != mr.OwnerXRKind {
			continue
		}
		if mr.OwnerXRGroup != "" && cand.Group != mr.OwnerXRGroup {
			continue
		}
		if cand.UID == "" || cand.UID != mr.OwnerXRUID {
			continue
		}
		switch {
		case cand.Namespace == "":
			cluster = append(cluster, cand)
		case mr.Namespace != "" && cand.Namespace == mr.Namespace:
			namespaced = append(namespaced, cand)
		}
	}
	if mr.Namespace != "" && len(namespaced) == 1 {
		return namespaced[0], true
	}
	if mr.Namespace != "" && len(namespaced) > 1 {
		return XRInfo{}, false
	}
	if len(cluster) == 1 {
		return cluster[0], true
	}
	return XRInfo{}, false
}

// findXRByResourceRef returns the unique XR whose resourceRefs include this MR.
func (s *MemoryStore) findXRByResourceRef(mr MRInfo) (XRInfo, bool) {
	var hits []XRInfo
	for _, xr := range s.xrs {
		if mr.Namespace != "" && xr.Namespace != "" && xr.Namespace != mr.Namespace {
			continue
		}
		for _, ref := range xr.ResourceRefs {
			if ref.Name != mr.Name {
				continue
			}
			if ref.Kind != "" && ref.Kind != mr.Kind {
				continue
			}
			if ref.APIVersion != "" {
				group, _, _ := strings.Cut(ref.APIVersion, "/")
				// Core apiVersion "v1" has no slash; treat as empty group.
				if !strings.Contains(ref.APIVersion, "/") {
					group = ""
				}
				if group != "" && group != mr.Group {
					continue
				}
			}
			hits = append(hits, xr)
			break
		}
	}
	if len(hits) != 1 {
		return XRInfo{}, false
	}
	return hits[0], true
}

// EnrichMRClaims resolves each MR's backing XR (via xr_name / composite label)
// and copies typed XR identity (kind, namespace, group). It also copies legacy
// claim linkage when claim fields are not already set from MR labels. It
// prefers an XR in the MR's namespace, then falls back to a cluster-scoped XR.
// When more than one XR matches (same name in the preferred scope, different
// kinds/groups), typed parent labels are left empty rather than guessing.
func (s *MemoryStore) EnrichMRClaims() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, mr := range s.mrs {
		if mr.XRName == "" {
			continue
		}
		xr, ok := s.lookupXR(mr.Namespace, mr.XRName)
		if !ok {
			// Clear stale typed identity when the XR is missing or ambiguous.
			if mr.XRKind != "" || mr.XRNamespace != "" || mr.XRGroup != "" {
				mr.XRKind = ""
				mr.XRNamespace = ""
				mr.XRGroup = ""
				s.mrs[key] = mr
			}
			continue
		}
		mr.XRKind = xr.Kind
		mr.XRNamespace = xr.Namespace
		mr.XRGroup = xr.Group
		if mr.ClaimName == "" {
			mr.ClaimName = xr.ClaimName
			mr.ClaimNS = xr.ClaimNS
		}
		s.mrs[key] = mr
	}
}

// lookupXR returns the single XR matching name in the preferred scope, or
// false when none or more than one match (ambiguous across kinds/groups).
// Preference: namespaced XRs in namespace when namespace is non-empty; otherwise
// (or when none are namespaced there) cluster-scoped XRs with that name.
func (s *MemoryStore) lookupXR(namespace, name string) (XRInfo, bool) {
	matches := s.matchXRs(namespace, name)
	if len(matches) != 1 {
		return XRInfo{}, false
	}
	return matches[0], true
}

// matchXRs returns XR candidates for a consumer in namespace looking up name.
func (s *MemoryStore) matchXRs(namespace, name string) []XRInfo {
	var namespaced, cluster []XRInfo
	for _, xr := range s.xrs {
		if xr.Name != name {
			continue
		}
		switch {
		case xr.Namespace == "":
			cluster = append(cluster, xr)
		case namespace != "" && xr.Namespace == namespace:
			namespaced = append(namespaced, xr)
		}
	}
	if namespace != "" && len(namespaced) > 0 {
		return namespaced
	}
	return cluster
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

// SnapshotPackages returns a copy of all stored packages.
func (s *MemoryStore) SnapshotPackages() []PackageInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]PackageInfo, 0, len(s.packages))
	for _, p := range s.packages {
		out = append(out, p)
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

// PackageCount returns the total number of stored packages.
func (s *MemoryStore) PackageCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.packages)
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

// xrKey produces a map key that includes GVR so distinct XR kinds sharing a
// namespace/name remain distinct entries.
func xrKey(gvr, namespace, name string) string {
	return gvr + "|" + objectKey(namespace, name)
}

func packageKey(kind, name string) string {
	return kind + "/" + name
}
