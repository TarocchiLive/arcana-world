package store

import "arcana-world/internal/domain"

// Options 选择凭据存储方式和仅在当前会话生效的配置覆盖项。
// CredentialBackend 为空时采用自动后端策略。
type Options struct {
	CredentialBackend string
	Overrides         ConfigOverrides
}

// ConfigOverrides 仅包含显式提供的会话设置。
// nil 字段保留已保存的设置。
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

// Apply 返回有效配置，其中的切片均独立持有。
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

// Overrides 返回此存储不可变会话覆盖项的独立副本。
func (s *Store) Overrides() ConfigOverrides {
	return s.overrides.detached()
}
