package chathistory_test

import (
	"sync"
	"testing"
	"time"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spec: HIST-001
func TestBuffer_Add_StoresFullEntry(t *testing.T) {
	t.Parallel()

	now := time.Now()
	buf := chathistory.NewBuffer(50)
	buf.Add(1, chathistory.Entry{
		ID: 10, Time: now, Author: "@alice", Text: "hello",
		ReplyToID: 0, FromBot: false,
	})
	buf.Add(1, chathistory.Entry{
		ID: 11, Time: now.Add(time.Second), Author: "бот", Text: "world",
		ReplyToID: 10, FromBot: true,
	})

	history := buf.Get(1)
	require.Len(t, history, 2)
	assert.Equal(t, 10, history[0].ID)
	assert.Equal(t, "@alice", history[0].Author)
	assert.Equal(t, "hello", history[0].Text)
	assert.Equal(t, 0, history[0].ReplyToID)
	assert.False(t, history[0].FromBot)
	assert.Equal(t, 11, history[1].ID)
	assert.Equal(t, "бот", history[1].Author)
	assert.Equal(t, 10, history[1].ReplyToID)
	assert.True(t, history[1].FromBot)
}

// spec: HIST-002
func TestBuffer_GetEmptyChat(t *testing.T) {
	t.Parallel()

	buf := chathistory.NewBuffer(50)
	history := buf.Get(999)
	assert.Empty(t, history)
}

// spec: HIST-003
func TestBuffer_IndependentChats(t *testing.T) {
	t.Parallel()

	buf := chathistory.NewBuffer(50)
	buf.Add(1, chathistory.Entry{ID: 1, Author: "alice", Text: "chat1"})
	buf.Add(2, chathistory.Entry{ID: 2, Author: "bob", Text: "chat2"})

	h1 := buf.Get(1)
	h2 := buf.Get(2)
	require.Len(t, h1, 1)
	require.Len(t, h2, 1)
	assert.Equal(t, "chat1", h1[0].Text)
	assert.Equal(t, "chat2", h2[0].Text)
}

// spec: HIST-004
func TestBuffer_Overflow(t *testing.T) {
	t.Parallel()

	buf := chathistory.NewBuffer(3)
	buf.Add(1, chathistory.Entry{ID: 1, Text: "msg1"})
	buf.Add(1, chathistory.Entry{ID: 2, Text: "msg2"})
	buf.Add(1, chathistory.Entry{ID: 3, Text: "msg3"})
	buf.Add(1, chathistory.Entry{ID: 4, Text: "msg4"})

	history := buf.Get(1)
	require.Len(t, history, 3)
	assert.Equal(t, "msg2", history[0].Text)
	assert.Equal(t, "msg4", history[2].Text)
}

// spec: HIST-007
func TestNewBuffer_PanicsOnZeroMaxSize(t *testing.T) {
	t.Parallel()

	assert.Panics(t, func() { chathistory.NewBuffer(0) })
}

// spec: HIST-007
func TestNewBuffer_PanicsOnNegativeMaxSize(t *testing.T) {
	t.Parallel()

	assert.Panics(t, func() { chathistory.NewBuffer(-1) })
}

// spec: HIST-005
func TestBuffer_AddAll_AppendsAdjacentEntries(t *testing.T) {
	t.Parallel()

	buf := chathistory.NewBuffer(50)
	buf.Add(1, chathistory.Entry{ID: 1, Author: "@alice", Text: "before"})
	buf.AddAll(1,
		chathistory.Entry{ID: 2, Author: "@alice", Text: "user turn"},
		chathistory.Entry{ID: 3, Author: "бот", Text: "bot turn", FromBot: true, ReplyToID: 2},
	)

	history := buf.Get(1)
	require.Len(t, history, 3)
	assert.Equal(t, "before", history[0].Text)
	assert.Equal(t, "user turn", history[1].Text)
	assert.Equal(t, "bot turn", history[2].Text)
	assert.True(t, history[2].FromBot)
}

// spec: HIST-004
func TestBuffer_AddAll_RespectsMaxSize(t *testing.T) {
	t.Parallel()

	buf := chathistory.NewBuffer(3)
	buf.Add(1, chathistory.Entry{ID: 1, Text: "a"})
	buf.AddAll(1,
		chathistory.Entry{ID: 2, Text: "b"},
		chathistory.Entry{ID: 3, Text: "c"},
		chathistory.Entry{ID: 4, Text: "d"},
	)

	history := buf.Get(1)
	require.Len(t, history, 3)
	assert.Equal(t, "b", history[0].Text)
	assert.Equal(t, "d", history[2].Text)
}

// spec: HIST-005
func TestBuffer_AddAll_EmptyArgs_NoOp(t *testing.T) {
	t.Parallel()

	buf := chathistory.NewBuffer(50)
	buf.AddAll(1)
	assert.Empty(t, buf.Get(1))
}

// spec: HIST-006
func TestBuffer_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	buf := chathistory.NewBuffer(50)
	var wg sync.WaitGroup

	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf.Add(1, chathistory.Entry{Author: "user", Text: "msg"})
			buf.Get(1)
		}()
	}

	wg.Wait()
	history := buf.Get(1)
	assert.LessOrEqual(t, len(history), 50)
}
