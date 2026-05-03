package bucketfill

import (
	"fmt"

	"github.com/n1kola-petrovic/bucketfill/provider/fs"
)

// providerOpener is the type of a registered factory for a non-FS provider.
// gcs/s3 register themselves at init time so the root package doesn't pull
// in their SDK dependencies unless the user actually imports them.
type providerOpener func(*Config) (ObjectStorage, error)

var providerOpeners = map[string]providerOpener{}

// RegisterProvider lets a provider package make itself reachable by name from
// OpenProvider. Called from each provider's init().
func RegisterProvider(name string, open providerOpener) {
	providerOpeners[name] = open
}

// OpenProvider builds the ObjectStorage implementation matching cfg.Provider.
// The "fs" provider is always available because it has no external deps; gcs
// and s3 must be linked in (e.g. by importing _ "github.com/.../provider/s3").
func OpenProvider(cfg *Config) (ObjectStorage, error) {
	switch cfg.Provider {
	case "fs":
		if cfg.FS == nil {
			return nil, fmt.Errorf("bucketfill: fs provider requires fs config")
		}
		return fs.New(cfg.FS.Root), nil
	default:
		open, ok := providerOpeners[cfg.Provider]
		if !ok {
			return nil, fmt.Errorf("bucketfill: provider %q is not linked in this binary; import the provider package to enable it", cfg.Provider)
		}
		return open(cfg)
	}
}

// EffectiveBucket returns the bucket identifier the provider should use. For
// fs, that's the literal bucket name (subfolder under FS.Root); for cloud
// providers it's the configured bucket. Always non-empty after Validate().
func (c *Config) EffectiveBucket() string {
	if c.Provider == "fs" && c.Bucket == "" {
		return "default"
	}
	return c.Bucket
}
