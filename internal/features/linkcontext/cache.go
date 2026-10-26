package linkcontext

import (
	"time"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
)

const (
	maxCacheEntries = 512
	positiveTTL     = time.Hour
	incompleteTTL   = 5 * time.Minute
	pageErrorTTL    = time.Minute
)

type cachedResult struct {
	fields    chathistory.Link
	expiresAt time.Time
}

type resultCache struct {
	records lru[string, cachedResult]
}

func newResultCache() resultCache {
	return resultCache{records: newLRU[string, cachedResult]()}
}

func (c *resultCache) get(key string, now time.Time) (chathistory.Link, bool) {
	entry, ok := c.records.entries[key]
	if !ok {
		return chathistory.Link{}, false
	}
	result := entry.value
	if !now.Before(result.expiresAt) {
		c.records.remove(entry)
		return chathistory.Link{}, false
	}
	c.records.touch(entry)
	return result.fields, true
}

func (c *resultCache) put(key string, fields chathistory.Link, now time.Time, ttl time.Duration) {
	c.records.put(key, cachedResult{fields: fields, expiresAt: now.Add(ttl)})
	if len(c.records.entries) > maxCacheEntries {
		c.records.remove(c.records.oldest)
	}
}
