package conversation

import (
	"strings"
	"sync"
	"time"
)

type Key struct {
	ChatID   int64
	ThreadID int64
}

type Message struct {
	Role string `json:"role"`
	Text string `json:"content"`
}

type entry struct {
	messages   []Message
	usedAt     time.Time
	generation uint64
	busy       bool
}

type Store struct {
	mu              sync.Mutex
	sequence        uint64
	entries         map[Key]*entry
	maxMessages     int
	maxEntries      int
	maxMessageRunes int
}

func NewStore(maxMessages, maxEntries, maxMessageRunes int) *Store {
	return &Store{
		entries:         make(map[Key]*entry),
		maxMessages:     maxMessages,
		maxEntries:      maxEntries,
		maxMessageRunes: maxMessageRunes,
	}
}

// TryLock serializes a scope and pins its entry until processing completes.
func (s *Store) TryLock(key Key) (func(), bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.entryLocked(key)
	if e == nil || e.busy {
		return nil, false
	}
	e.busy = true
	e.usedAt = time.Now()
	return func() {
		s.mu.Lock()
		e.busy = false
		s.mu.Unlock()
	}, true
}

// Each entry and reset receives a unique generation, including after eviction.
func (s *Store) entryLocked(key Key) *entry {
	if e := s.entries[key]; e != nil {
		return e
	}
	if !s.evictOldestLocked() {
		return nil
	}
	s.sequence++
	e := &entry{generation: s.sequence, usedAt: time.Now()}
	s.entries[key] = e
	return e
}

func (s *Store) History(key Key) []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.entries[key]
	if e == nil {
		return nil
	}
	e.usedAt = time.Now()
	out := make([]Message, len(e.messages))
	copy(out, e.messages)
	return out
}

func (s *Store) Generation(key Key) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.entryLocked(key)
	if e == nil {
		return 0
	}
	return e.generation
}

func (s *Store) Append(key Key, messages ...Message) {
	s.append(key, 0, false, messages...)
}

func (s *Store) AppendIfGeneration(key Key, generation uint64, messages ...Message) bool {
	return s.append(key, generation, true, messages...)
}

func (s *Store) append(key Key, generation uint64, conditional bool, messages ...Message) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.entries[key]
	if e == nil {
		if conditional {
			return false
		}
		e = s.entryLocked(key)
		if e == nil {
			return false
		}
	}
	if conditional && e.generation != generation {
		return false
	}
	for _, message := range messages {
		message.Text = clipRunes(strings.TrimSpace(message.Text), s.maxMessageRunes)
		if message.Text == "" || (message.Role != "user" && message.Role != "assistant") {
			continue
		}
		e.messages = append(e.messages, message)
	}
	if len(e.messages) > s.maxMessages {
		e.messages = append([]Message(nil), e.messages[len(e.messages)-s.maxMessages:]...)
	}
	e.usedAt = time.Now()
	return true
}

func (s *Store) Reset(key Key) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.entryLocked(key)
	if e == nil {
		return
	}
	e.messages = nil
	s.sequence++
	e.generation = s.sequence
	e.usedAt = time.Now()
}

func (s *Store) evictOldestLocked() bool {
	if len(s.entries) < s.maxEntries {
		return true
	}
	var oldestKey Key
	var oldest *entry
	for key, candidate := range s.entries {
		if candidate.busy {
			continue
		}
		if oldest == nil || candidate.usedAt.Before(oldest.usedAt) {
			oldestKey, oldest = key, candidate
		}
	}
	if oldest != nil {
		delete(s.entries, oldestKey)
		return true
	}
	return false
}

func clipRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
