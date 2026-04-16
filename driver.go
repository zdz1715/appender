package appender

import (
	"context"
	"io"
	"time"
)

type Metadata struct {
	Path       string     `json:"path"`
	Expiration *time.Time `json:"expiration"`
}

type Driver interface {
	Get(ctx context.Context, id string) (*Metadata, error)

	GetContent(ctx context.Context, id string) (io.ReadCloser, error)

	Delete(ctx context.Context, id string) error

	Append(ctx context.Context, id string, data []byte, offset int64) error
}
