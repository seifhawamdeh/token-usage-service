package adapter

import (
	"context"

	"github.com/seif/token-usage-service/internal/model"
)

// VendorAdapter discovers and parses one vendor's on-disk sources.
type VendorAdapter interface {
	Name() string
	Version() string
	Discover(ctx context.Context) ([]model.SourceDescriptor, error)
	Parse(ctx context.Context, src model.SourceDescriptor) model.ParseResult
}
