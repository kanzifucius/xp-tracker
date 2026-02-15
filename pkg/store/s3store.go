package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// maxSnapshotSize is the maximum allowed size for an S3 snapshot (100 MiB).
// This prevents unbounded memory allocation if the snapshot object is
// corrupted or maliciously large.
const maxSnapshotSize = 100 << 20

// S3Client is the subset of the AWS S3 client API used by S3Store.
// It exists to allow dependency injection of a mock in tests.
type S3Client interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

// S3Store wraps a MemoryStore and adds S3 persistence.
// All Store methods delegate to the embedded MemoryStore so reads are
// always fast (served from memory). Persist serialises the current
// in-memory state to a single S3 object; Restore re-hydrates the
// MemoryStore from S3 on startup.
type S3Store struct {
	mem    *MemoryStore
	client S3Client
	bucket string
	key    string

	// persistMu serialises Persist calls so concurrent poll cycles
	// (shouldn't happen, but defensive) don't race on S3 writes.
	persistMu sync.Mutex
}

// NewS3Store creates an S3Store that persists snapshots to
// s3://<bucket>/<keyPrefix>/snapshot.json.
func NewS3Store(mem *MemoryStore, client S3Client, bucket, keyPrefix string) *S3Store {
	return &S3Store{
		mem:    mem,
		client: client,
		bucket: bucket,
		key:    keyPrefix + "/snapshot.json",
	}
}

// ---------------------------------------------------------------------------
// Store interface delegation – all reads/writes go through MemoryStore.
// ---------------------------------------------------------------------------

func (s *S3Store) ReplaceClaims(gvr string, items []ClaimInfo) { s.mem.ReplaceClaims(gvr, items) }
func (s *S3Store) ReplaceXRs(gvr string, items []XRInfo)       { s.mem.ReplaceXRs(gvr, items) }
func (s *S3Store) EnrichClaimCompositions()                    { s.mem.EnrichClaimCompositions() }
func (s *S3Store) SnapshotClaims() []ClaimInfo                 { return s.mem.SnapshotClaims() }
func (s *S3Store) SnapshotXRs() []XRInfo                       { return s.mem.SnapshotXRs() }
func (s *S3Store) ClaimCount() int                             { return s.mem.ClaimCount() }
func (s *S3Store) XRCount() int                                { return s.mem.XRCount() }

// ---------------------------------------------------------------------------
// PersistentStore implementation
// ---------------------------------------------------------------------------

// Persist serialises the current in-memory state to S3 as JSON.
func (s *S3Store) Persist(ctx context.Context) error {
	s.persistMu.Lock()
	defer s.persistMu.Unlock()

	snap := Snapshot{
		Claims:      s.mem.SnapshotClaims(),
		XRs:         s.mem.SnapshotXRs(),
		PersistedAt: time.Now().UTC(),
	}

	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}

	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      &s.bucket,
		Key:         &s.key,
		Body:        bytes.NewReader(data),
		ContentType: strPtr("application/json"),
	})
	if err != nil {
		return err
	}

	slog.Debug("persisted store snapshot to S3",
		"bucket", s.bucket,
		"key", s.key,
		"claims", len(snap.Claims),
		"xrs", len(snap.XRs),
	)
	return nil
}

// Restore loads a snapshot from S3 and replays it into the MemoryStore.
// If the S3 key does not exist the store starts empty (no error).
// Any other S3 error is returned so the caller can decide how to handle it.
func (s *S3Store) Restore(ctx context.Context) error {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    &s.key,
	})
	if err != nil {
		// NoSuchKey → start with empty store.
		var noKey *types.NoSuchKey
		if errors.As(err, &noKey) {
			slog.Warn("no existing S3 snapshot found, starting with empty store",
				"bucket", s.bucket,
				"key", s.key,
			)
			return nil
		}
		return err
	}
	defer func() { _ = out.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(out.Body, maxSnapshotSize+1))
	if err != nil {
		return err
	}
	if len(data) > maxSnapshotSize {
		return fmt.Errorf("S3 snapshot exceeds maximum allowed size of %d bytes", maxSnapshotSize)
	}

	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}

	// Group claims by GVR and replay into MemoryStore so that
	// per-GVR stale removal works correctly.
	claimsByGVR := make(map[string][]ClaimInfo)
	for _, c := range snap.Claims {
		claimsByGVR[c.GVR] = append(claimsByGVR[c.GVR], c)
	}
	for gvr, items := range claimsByGVR {
		s.mem.ReplaceClaims(gvr, items)
	}

	xrsByGVR := make(map[string][]XRInfo)
	for _, x := range snap.XRs {
		xrsByGVR[x.GVR] = append(xrsByGVR[x.GVR], x)
	}
	for gvr, items := range xrsByGVR {
		s.mem.ReplaceXRs(gvr, items)
	}

	slog.Info("restored store snapshot from S3",
		"bucket", s.bucket,
		"key", s.key,
		"claims", len(snap.Claims),
		"xrs", len(snap.XRs),
		"persistedAt", snap.PersistedAt,
	)
	return nil
}

// ---------------------------------------------------------------------------
// S3 client factory
// ---------------------------------------------------------------------------

// NewS3Client creates a real AWS S3 client using default credential chain.
// If endpoint is non-empty, path-style addressing is enabled (for MinIO, LocalStack, etc.).
func NewS3Client(ctx context.Context, region, endpoint string) (*s3.Client, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, err
	}

	opts := []func(*s3.Options){}
	if endpoint != "" {
		opts = append(opts, func(o *s3.Options) {
			o.BaseEndpoint = &endpoint
			o.UsePathStyle = true
		})
	}

	return s3.NewFromConfig(awsCfg, opts...), nil
}

func strPtr(s string) *string { return &s }
