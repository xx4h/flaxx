package checker

import "github.com/xx4h/flaxx/internal/cache"

// activeCache is consulted by FetchHelmVersions / FetchOCIVersions /
// FetchImageTags. A nil value disables caching (Get/Set become no-ops).
var activeCache *cache.Cache

// SetCache installs c as the package-level cache for registry lookups.
// Pass nil to disable caching.
func SetCache(c *cache.Cache) { activeCache = c }

// cacheKey prefixes are stable strings used when computing cache keys.
// They also namespace entries on disk — handy when inspecting the cache.
const (
	cachePrefixHelm = "helm" // helm standard-repo index.yaml lookups
	cachePrefixOCI  = "oci"  // OCI tags/list lookups (helm + images)
)
