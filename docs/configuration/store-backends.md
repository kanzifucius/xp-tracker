# Store Backends

xp-tracker uses a pluggable store interface for holding claim and XR metadata in memory. By default, data lives only in memory and is lost on restart. For workloads that restart frequently, an S3 persistent backend is available.

## Store interface

The `store.Store` interface defines 7 methods:

```go
type Store interface {
    ReplaceClaims(gvr string, items []ClaimInfo)
    ReplaceXRs(gvr string, items []XRInfo)
    EnrichClaimCompositions()
    SnapshotClaims() []ClaimInfo
    SnapshotXRs() []XRInfo
    ClaimCount() int
    XRCount() int
}
```

All implementations must be safe for concurrent use.

## Memory store (default)

The default `MemoryStore` is a thread-safe in-memory store backed by Go maps and a `sync.RWMutex`. It requires no configuration.

```bash
STORE_BACKEND=memory  # or simply omit the variable
```

**Trade-offs:**

- Fast reads and writes
- No external dependencies
- Data is lost on pod restart -- the next poll cycle repopulates it (default: 30 seconds)

## S3 persistent store

The `S3Store` wraps `MemoryStore` with S3 persistence using a decorator pattern. All reads are served from memory (so Prometheus scraping stays fast). After each poll cycle, the in-memory snapshot is serialised to S3. On startup, the store restores from S3 before the first poll.

```bash
STORE_BACKEND=s3
S3_BUCKET=my-xp-tracker-bucket
S3_KEY_PREFIX=xp-tracker          # optional, default: xp-tracker
S3_REGION=us-east-1               # optional, default: us-east-1
S3_ENDPOINT=http://minio:9000     # optional, for S3-compatible providers
```

### How it works

1. **Startup**: attempts to restore from `s3://<bucket>/<prefix>/snapshot.json`
2. **Each poll cycle**: writes the full snapshot to the same S3 key (overwrite)
3. **If S3 is unreachable at startup**: starts with an empty store and logs a warning

### Authentication

The S3 client uses the [AWS SDK v2 default credential chain](https://docs.aws.amazon.com/sdk-for-go/v2/developer-guide/configuring-sdk.html), which supports:

- IAM roles for service accounts (IRSA) -- recommended for EKS
- Environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`)
- Shared credentials file (`~/.aws/credentials`)
- EC2 instance metadata

### S3-compatible providers

Set `S3_ENDPOINT` to use MinIO, LocalStack, or any S3-compatible API:

```bash
S3_ENDPOINT=http://minio.minio.svc:9000
```

Path-style addressing is automatically enabled when a custom endpoint is set.

### Snapshot format

The snapshot is a single JSON file containing all claims and XRs:

```json
{
  "claims": [...],
  "xrs": [...],
  "persistedAt": "2026-02-15T10:00:00Z"
}
```

## Implementing a custom backend

To add a new persistent backend (e.g., DynamoDB, PostgreSQL), implement the `PersistentStore` interface:

```go
type PersistentStore interface {
    Store
    Persist(ctx context.Context) error  // called after each poll cycle
    Restore(ctx context.Context) error  // called once at startup
}
```

The recommended approach is the decorator pattern: wrap `MemoryStore`, delegate all `Store` methods to it, and add persistence in `Persist`/`Restore`. See `pkg/store/s3store.go` for a reference implementation.
