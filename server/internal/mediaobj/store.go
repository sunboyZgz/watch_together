package mediaobj

import (
	"context"
	"io"
	"time"
)

// ObjectStore abstracts media object storage behind playback delivery modes.
type ObjectStore interface {
	GetObject(ctx context.Context, key string) (io.ReadCloser, error)
	PresignGetObject(ctx context.Context, key string, ttl time.Duration) (string, error)
}
