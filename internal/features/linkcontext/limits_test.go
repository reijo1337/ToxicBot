package linkcontext

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// spec: LINK-023
func TestLimitsRejectBurstsAndRefill(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	limits := NewLimits()
	limits.now = func() time.Time { return now }

	require.True(t, limits.allowChat(1))
	require.True(t, limits.allowChat(1))
	require.False(t, limits.allowChat(1))
	require.True(t, limits.allowChat(2))
	now = now.Add(10 * time.Second)
	require.True(t, limits.allowChat(1))
	require.False(t, limits.allowChat(1))

	for range 8 {
		require.True(t, limits.AllowHTTP("EXAMPLE.COM."))
	}
	require.False(t, limits.AllowHTTP("example.com"))
	now = now.Add(time.Second)
	require.False(t, limits.AllowHTTP("example.com"))
	require.True(t, limits.AllowHTTP("other.example"))
	require.False(t, limits.AllowHTTP("third.example"))
	now = now.Add(4 * time.Second)
	require.True(t, limits.AllowHTTP("example.com"))

	require.True(t, limits.allowVision())
	require.True(t, limits.allowVision())
	require.False(t, limits.allowVision())
	now = now.Add(6 * time.Second)
	require.True(t, limits.allowVision())
	require.False(t, limits.allowVision())
}

// spec: LINK-023
func TestLimiterMapsBoundIdleAndKeepGlobalBudget(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	limits := NewLimits()
	limits.now = func() time.Time { return now }

	for index := range 513 {
		require.True(t, limits.allowChat(int64(index)))
		now = now.Add(time.Second)
		require.True(t, limits.AllowHTTP(strconv.Itoa(index)+".example"))
	}
	require.Len(t, limits.chats.records.entries, 512)
	require.Len(t, limits.hosts.records.entries, 512)
	_, exists := limits.chats.records.entries[0]
	require.False(t, exists)

	for range 7 {
		require.True(t, limits.AllowHTTP("fresh.example"))
	}
	require.False(t, limits.AllowHTTP("another.example"))

	now = now.Add(10*time.Minute + time.Second)
	require.True(t, limits.allowChat(999))
	require.True(t, limits.AllowHTTP("only.example"))
	require.Len(t, limits.chats.records.entries, 1)
	require.Len(t, limits.hosts.records.entries, 1)
}
