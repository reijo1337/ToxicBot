package linkcontext

type lruEntry[K comparable, V any] struct {
	key   K
	value V
	newer *lruEntry[K, V]
	older *lruEntry[K, V]
}

type lru[K comparable, V any] struct {
	entries map[K]*lruEntry[K, V]
	newest  *lruEntry[K, V]
	oldest  *lruEntry[K, V]
}

func newLRU[K comparable, V any]() lru[K, V] {
	return lru[K, V]{entries: make(map[K]*lruEntry[K, V])}
}

func (l *lru[K, V]) touch(entry *lruEntry[K, V]) {
	if entry == l.newest {
		return
	}
	l.detach(entry)
	l.prepend(entry)
}

func (l *lru[K, V]) put(key K, value V) {
	if entry, ok := l.entries[key]; ok {
		entry.value = value
		l.touch(entry)
		return
	}
	entry := &lruEntry[K, V]{key: key, value: value}
	l.entries[key] = entry
	l.prepend(entry)
}

func (l *lru[K, V]) remove(entry *lruEntry[K, V]) {
	l.detach(entry)
	delete(l.entries, entry.key)
}

func (l *lru[K, V]) prepend(entry *lruEntry[K, V]) {
	entry.newer = nil
	entry.older = l.newest
	if l.newest != nil {
		l.newest.newer = entry
	} else {
		l.oldest = entry
	}
	l.newest = entry
}

func (l *lru[K, V]) detach(entry *lruEntry[K, V]) {
	if entry.newer != nil {
		entry.newer.older = entry.older
	} else {
		l.newest = entry.older
	}
	if entry.older != nil {
		entry.older.newer = entry.newer
	} else {
		l.oldest = entry.newer
	}
}
