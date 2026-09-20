package store

import "arcana-world/internal/domain"

// Options selects credential storage and session-only configuration overrides.
// An empty CredentialBackend selects the automatic backend policy.
type Options struct {
	CredentialBackend string
	Overrides         ConfigOverrides
}

// ConfigOverrides contains only explicitly supplied session settings.
// Nil fields leave the saved setting unchanged.
type ConfigOverrides struct {
	Proxy          *string
	OBSAutoConnect *bool
	OBSAutoStream  *bool
}

func (o ConfigOverrides) detached() ConfigOverrides {
	if o.Proxy != nil {
		value := *o.Proxy
		o.Proxy = &value
	}
	if o.OBSAutoConnect != nil {
		value := *o.OBSAutoConnect
		o.OBSAutoConnect = &value
	}
	if o.OBSAutoStream != nil {
		value := *o.OBSAutoStream
		o.OBSAutoStream = &value
	}
	return o
}

// Apply returns an effective configuration with independently owned slices.
func (o ConfigOverrides) Apply(c domain.Config) domain.Config {
	c = clone(c)
	if o.Proxy != nil {
		c.Proxy = *o.Proxy
	}
	if o.OBSAutoConnect != nil {
		c.OBSAutoConnect = *o.OBSAutoConnect
	}
	if o.OBSAutoStream != nil {
		c.OBSAutoStream = *o.OBSAutoStream
	}
	return c
}

// Overrides returns a detached copy of this store's immutable session overrides.
func (s *Store) Overrides() ConfigOverrides {
	return s.overrides.detached()
}
