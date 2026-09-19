package serve

import "gooru.local/internal/filesource"

// newComposedMediaServiceFromConfig is the server composition boundary for
// logical media access. Raw encryption key material is consumed here to build
// the storage capability, then removed before the runtime config reaches media
// feature code.
func newComposedMediaServiceFromConfig(cfg Config) *MediaService {
	resolver, resolverErr := newMediaSourceResolver(cfg)
	return newComposedMediaService(cfg, resolver, resolverErr)
}

// newComposedMediaService constructs the media service from already-resolved
// logical source, derivative-persistence, and thumbnail-access policies. The
// sanitized runtime config deliberately excludes raw encryption key material so
// media feature code cannot acquire the master key through its general-purpose
// config object.
func newComposedMediaService(cfg Config, resolver *filesource.Resolver, resolverErr error) *MediaService {
	store, storeErr := newDerivativeStore(cfg)
	thumbnailGeneration := newThumbnailGenerationPolicy(cfg.Encryption.Enabled)
	thumbnailQualityGeneration := newThumbnailQualityGenerationPolicy(cfg.Encryption.Enabled)
	runtimeCfg := cfg
	runtimeCfg.Encryption.Key = nil
	return &MediaService{
		cfg:                        runtimeCfg,
		thumbnailer:                NewMediaThumbnailer(runtimeCfg),
		thumbnailGeneration:        thumbnailGeneration,
		thumbnailQualityGeneration: thumbnailQualityGeneration,
		sourceResolver:             resolver,
		sourceResolverErr:          resolverErr,
		derivatives:                store,
		derivativeStoreErr:         storeErr,
	}
}
