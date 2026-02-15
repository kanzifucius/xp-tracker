package store

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"sort"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// ---------------------------------------------------------------------------
// Compile-time interface checks
// ---------------------------------------------------------------------------

var (
	_ Store           = (*S3Store)(nil)
	_ PersistentStore = (*S3Store)(nil)
)

// ---------------------------------------------------------------------------
// Mock S3 client
// ---------------------------------------------------------------------------

type mockS3Client struct {
	objects map[string][]byte // key → body
	putErr  error
	getErr  error
}

func newMockS3Client() *mockS3Client {
	return &mockS3Client{objects: make(map[string][]byte)}
}

func (m *mockS3Client) PutObject(_ context.Context, input *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if m.putErr != nil {
		return nil, m.putErr
	}
	data, err := io.ReadAll(input.Body)
	if err != nil {
		return nil, err
	}
	m.objects[*input.Key] = data
	return &s3.PutObjectOutput{}, nil
}

func (m *mockS3Client) GetObject(_ context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	data, ok := m.objects[*input.Key]
	if !ok {
		return nil, &types.NoSuchKey{}
	}
	return &s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader(data)),
	}, nil
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestS3Store_PersistAndRestore(t *testing.T) {
	mem := New()
	mock := newMockS3Client()
	ss := NewS3Store(mem, mock, "my-bucket", "prefix")

	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	ss.ReplaceClaims("g1/v1/claims", []ClaimInfo{
		{GVR: "g1/v1/claims", Group: "g1", Kind: "Claim", Namespace: "ns1", Name: "c1", Ready: true, CreatedAt: now},
		{GVR: "g1/v1/claims", Group: "g1", Kind: "Claim", Namespace: "ns2", Name: "c2", Ready: false, CreatedAt: now},
	})
	ss.ReplaceXRs("g1/v1/xrs", []XRInfo{
		{GVR: "g1/v1/xrs", Group: "g1", Kind: "XR", Name: "xr1", Composition: "comp-a", Ready: true, CreatedAt: now},
	})

	ctx := context.Background()

	// Persist to mock S3.
	if err := ss.Persist(ctx); err != nil {
		t.Fatalf("Persist failed: %v", err)
	}

	// Verify the snapshot was written.
	if _, ok := mock.objects["prefix/snapshot.json"]; !ok {
		t.Fatal("expected snapshot.json in mock S3")
	}

	// Create a fresh MemoryStore + S3Store and restore.
	mem2 := New()
	ss2 := NewS3Store(mem2, mock, "my-bucket", "prefix")

	if err := ss2.Restore(ctx); err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	if ss2.ClaimCount() != 2 {
		t.Errorf("expected 2 claims after restore, got %d", ss2.ClaimCount())
	}
	if ss2.XRCount() != 1 {
		t.Errorf("expected 1 XR after restore, got %d", ss2.XRCount())
	}

	// Verify field fidelity.
	claims := ss2.SnapshotClaims()
	sort.Slice(claims, func(i, j int) bool { return claims[i].Name < claims[j].Name })
	if claims[0].Namespace != "ns1" || claims[0].Name != "c1" || !claims[0].Ready {
		t.Errorf("claim 0 field mismatch: %+v", claims[0])
	}
	if claims[1].Namespace != "ns2" || claims[1].Name != "c2" || claims[1].Ready {
		t.Errorf("claim 1 field mismatch: %+v", claims[1])
	}

	xrs := ss2.SnapshotXRs()
	if xrs[0].Composition != "comp-a" || xrs[0].Name != "xr1" {
		t.Errorf("XR 0 field mismatch: %+v", xrs[0])
	}
}

func TestS3Store_RestoreEmpty(t *testing.T) {
	mem := New()
	mock := newMockS3Client() // no objects stored
	ss := NewS3Store(mem, mock, "my-bucket", "prefix")

	ctx := context.Background()
	if err := ss.Restore(ctx); err != nil {
		t.Fatalf("Restore on missing key should not error, got: %v", err)
	}
	if ss.ClaimCount() != 0 {
		t.Errorf("expected 0 claims, got %d", ss.ClaimCount())
	}
	if ss.XRCount() != 0 {
		t.Errorf("expected 0 XRs, got %d", ss.XRCount())
	}
}

func TestS3Store_DelegatesAllMethods(t *testing.T) {
	mem := New()
	mock := newMockS3Client()
	ss := NewS3Store(mem, mock, "b", "p")

	now := time.Now()
	ss.ReplaceClaims("g/v/r", []ClaimInfo{
		{GVR: "g/v/r", Group: "g", Kind: "K", Namespace: "ns", Name: "a", XRRef: "xr1", CreatedAt: now},
	})
	ss.ReplaceXRs("g/v/xr", []XRInfo{
		{GVR: "g/v/xr", Group: "g", Kind: "XK", Name: "xr1", Composition: "comp", CreatedAt: now},
	})

	if ss.ClaimCount() != 1 {
		t.Errorf("ClaimCount: expected 1, got %d", ss.ClaimCount())
	}
	if ss.XRCount() != 1 {
		t.Errorf("XRCount: expected 1, got %d", ss.XRCount())
	}

	// EnrichClaimCompositions should work through delegation.
	ss.EnrichClaimCompositions()
	claims := ss.SnapshotClaims()
	if claims[0].Composition != "comp" {
		t.Errorf("EnrichClaimCompositions via delegation failed: got composition %q", claims[0].Composition)
	}

	xrs := ss.SnapshotXRs()
	if xrs[0].Kind != "XK" {
		t.Errorf("SnapshotXRs delegation failed: got kind %q", xrs[0].Kind)
	}
}

func TestS3Store_PersistPreservesGVR(t *testing.T) {
	mem := New()
	mock := newMockS3Client()
	ss := NewS3Store(mem, mock, "b", "p")

	ss.ReplaceClaims("g1/v1/r1", []ClaimInfo{
		{GVR: "g1/v1/r1", Group: "g1", Kind: "K1", Namespace: "ns", Name: "a"},
	})
	ss.ReplaceClaims("g2/v1/r2", []ClaimInfo{
		{GVR: "g2/v1/r2", Group: "g2", Kind: "K2", Namespace: "ns", Name: "b"},
	})

	ctx := context.Background()
	if err := ss.Persist(ctx); err != nil {
		t.Fatalf("Persist: %v", err)
	}

	// Verify the snapshot preserves distinct GVRs.
	var snap Snapshot
	if err := json.Unmarshal(mock.objects["p/snapshot.json"], &snap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	gvrs := make(map[string]bool)
	for _, c := range snap.Claims {
		gvrs[c.GVR] = true
	}
	if !gvrs["g1/v1/r1"] || !gvrs["g2/v1/r2"] {
		t.Errorf("expected both GVRs preserved, got %v", gvrs)
	}
}

func TestS3Store_RestoreMultiGVR(t *testing.T) {
	mem := New()
	mock := newMockS3Client()
	ss := NewS3Store(mem, mock, "b", "p")

	// Populate two GVRs of claims and two of XRs, then persist.
	ss.ReplaceClaims("g1/v1/r1", []ClaimInfo{
		{GVR: "g1/v1/r1", Group: "g1", Kind: "K1", Namespace: "ns", Name: "a"},
	})
	ss.ReplaceClaims("g2/v1/r2", []ClaimInfo{
		{GVR: "g2/v1/r2", Group: "g2", Kind: "K2", Namespace: "ns", Name: "b"},
	})
	ss.ReplaceXRs("g1/v1/xr1", []XRInfo{
		{GVR: "g1/v1/xr1", Group: "g1", Kind: "XK1", Name: "xr-a"},
	})
	ss.ReplaceXRs("g2/v1/xr2", []XRInfo{
		{GVR: "g2/v1/xr2", Group: "g2", Kind: "XK2", Name: "xr-b"},
	})

	ctx := context.Background()
	if err := ss.Persist(ctx); err != nil {
		t.Fatalf("Persist: %v", err)
	}

	// Restore into a fresh store.
	mem2 := New()
	ss2 := NewS3Store(mem2, mock, "b", "p")
	if err := ss2.Restore(ctx); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if ss2.ClaimCount() != 2 {
		t.Errorf("expected 2 claims, got %d", ss2.ClaimCount())
	}
	if ss2.XRCount() != 2 {
		t.Errorf("expected 2 XRs, got %d", ss2.XRCount())
	}

	// Now replace one GVR with empty — only that GVR's entries should be removed.
	ss2.ReplaceClaims("g1/v1/r1", nil)
	if ss2.ClaimCount() != 1 {
		t.Errorf("expected 1 claim after removing g1/v1/r1, got %d", ss2.ClaimCount())
	}

	claims := ss2.SnapshotClaims()
	if claims[0].GVR != "g2/v1/r2" {
		t.Errorf("expected remaining claim to be g2/v1/r2, got %s", claims[0].GVR)
	}
}

func TestS3Store_PersistError(t *testing.T) {
	mem := New()
	mock := newMockS3Client()
	mock.putErr = io.ErrClosedPipe // simulate S3 failure
	ss := NewS3Store(mem, mock, "b", "p")

	err := ss.Persist(context.Background())
	if err == nil {
		t.Fatal("expected Persist to return error on S3 failure")
	}
}

func TestS3Store_RestoreGetError(t *testing.T) {
	mem := New()
	mock := newMockS3Client()
	mock.getErr = io.ErrUnexpectedEOF // simulate non-NoSuchKey error
	ss := NewS3Store(mem, mock, "b", "p")

	err := ss.Restore(context.Background())
	if err == nil {
		t.Fatal("expected Restore to return error on non-NoSuchKey failure")
	}
}
