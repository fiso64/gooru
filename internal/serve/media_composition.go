package serve

import "gooru.local/internal/filesource"

// newComposedMediaService constructs the media service from an already-resolved
// logical source policy. The sanitized runtime config deliberately excludes raw
// encryption key material so media feature code cannot acquire the master key
// through its general-purpose config object.
func newComposedMediaService(cfg Config, resolver *filesource.Resolver, resolverErr error) *MediaService {
	runtimeCfg := cfg
	runtimeCfg.Encryption.Key = nil
	return &MediaService{
		cfg:               runtimeCfg,
		thumbnailer:       NewMediaThumbnailer(runtimeCfg),
		sourceResolver:    resolver,
		sourceResolverErr: resolverErr,
	}
}
