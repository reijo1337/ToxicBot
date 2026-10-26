package linkcontext

import (
	"strings"
	"sync"
	"time"
)

const (
	chatRequestsPerMinute   = 6
	chatBurst               = 2
	httpRequestsPerMinute   = 60
	httpBurst               = 8
	hostRequestsPerMinute   = 12
	hostBurst               = 8
	maxConcurrentVision     = 2
	visionRequestsPerMinute = 10
	visionBurst             = 2
	maxLimiterEntries       = 512
	limiterIdleTTL          = 10 * time.Minute
)

// Limits shares the request budgets between enrichment and every HTTP redirect hop.
type Limits struct {
	mu          sync.Mutex
	now         func() time.Time
	visionSlots chan struct{}
	http        bucket
	vision      bucket
	chats       bucketMap[int64]
	hosts       bucketMap[string]
}

type bucket struct {
	updated time.Time
	tokens  float64
	rate    float64
	burst   float64
}

type bucketRecord struct {
	lastUsed time.Time
	budget   bucket
}

type bucketMap[K comparable] struct {
	records lru[K, bucketRecord]
	rate    float64
	burst   float64
}

func NewLimits() *Limits {
	return &Limits{
		now: time.Now,
		// Each token belongs to a running photo or preview call in this process.
		visionSlots: make(chan struct{}, maxConcurrentVision),
		http:        newBucket(httpRequestsPerMinute, httpBurst),
		vision:      newBucket(visionRequestsPerMinute, visionBurst),
		chats:       newBucketMap[int64](chatRequestsPerMinute, chatBurst),
		hosts:       newBucketMap[string](hostRequestsPerMinute, hostBurst),
	}
}

// AllowHTTP reserves both quotas atomically. A denied host does not spend a global token.
func (l *Limits) AllowHTTP(hostname string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.http.refill(now)
	if l.http.tokens < 1 {
		return false
	}

	hostname = strings.TrimSuffix(strings.ToLower(hostname), ".")
	host := l.hosts.get(hostname, now)
	host.refill(now)
	if host.tokens < 1 {
		return false
	}
	l.http.tokens--
	host.tokens--
	return true
}

func (l *Limits) allowChat(chatID int64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	return l.chats.get(chatID, now).allow(now)
}

func (l *Limits) allowVision() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.vision.allow(l.now())
}

func newBucket(rate, burst float64) bucket {
	return bucket{tokens: burst, rate: rate / time.Minute.Seconds(), burst: burst}
}

func (b *bucket) refill(now time.Time) {
	if !b.updated.IsZero() && now.After(b.updated) {
		b.tokens = min(b.burst, b.tokens+now.Sub(b.updated).Seconds()*b.rate)
	}
	b.updated = now
}

func (b *bucket) allow(now time.Time) bool {
	b.refill(now)
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func newBucketMap[K comparable](rate, burst float64) bucketMap[K] {
	return bucketMap[K]{records: newLRU[K, bucketRecord](), rate: rate, burst: burst}
}

func (m *bucketMap[K]) get(key K, now time.Time) *bucket {
	m.expire(now)
	if entry, ok := m.records.entries[key]; ok {
		m.records.touch(entry)
		entry.value.lastUsed = now
		return &entry.value.budget
	}

	if len(m.records.entries) >= maxLimiterEntries {
		m.records.remove(m.records.oldest)
	}
	m.records.put(key, bucketRecord{
		lastUsed: now, budget: newBucket(m.rate, m.burst),
	})
	return &m.records.entries[key].value.budget
}

func (m *bucketMap[K]) expire(now time.Time) {
	for oldest := m.records.oldest; oldest != nil; oldest = m.records.oldest {
		if now.Sub(oldest.value.lastUsed) < limiterIdleTTL {
			return
		}
		m.records.remove(oldest)
	}
}
