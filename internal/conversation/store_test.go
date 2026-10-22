package conversation

import "testing"

func TestStoreScopesHistoryByChatAndTopic(t *testing.T) {
	s := NewStore(20, 8, 100)
	first := Key{ChatID: 1, ThreadID: 10}
	second := Key{ChatID: 1, ThreadID: 11}
	s.Append(first, Message{Role: "user", Text: "first"})
	s.Append(second, Message{Role: "user", Text: "second"})
	if got := s.History(first); len(got) != 1 || got[0].Text != "first" {
		t.Fatalf("first history = %#v", got)
	}
	if got := s.History(second); len(got) != 1 || got[0].Text != "second" {
		t.Fatalf("second history = %#v", got)
	}
}

func TestStoreKeepsLastTwentyBoundedMessages(t *testing.T) {
	s := NewStore(20, 8, 4)
	key := Key{ChatID: 1}
	for i := 0; i < 25; i++ {
		s.Append(key, Message{Role: "user", Text: "abcdef"})
	}
	got := s.History(key)
	if len(got) != 20 {
		t.Fatalf("history length = %d", len(got))
	}
	if got[0].Text != "abcd" {
		t.Fatalf("message was not clipped: %#v", got[0])
	}
	s.Reset(key)
	if got := s.History(key); len(got) != 0 {
		t.Fatalf("history survived reset: %#v", got)
	}
}

func TestStoreEvictsOldestConversation(t *testing.T) {
	s := NewStore(20, 2, 100)
	a := Key{ChatID: 1}
	b := Key{ChatID: 2}
	c := Key{ChatID: 3}
	s.Append(a, Message{Role: "user", Text: "a"})
	s.Append(b, Message{Role: "user", Text: "b"})
	s.Append(c, Message{Role: "user", Text: "c"})
	if got := s.History(a); len(got) != 0 {
		t.Fatalf("oldest conversation not evicted: %#v", got)
	}
}

func TestResetRejectsPreResetAppend(t *testing.T) {
	s := NewStore(20, 8, 100)
	key := Key{ChatID: 1}
	generation := s.Generation(key)
	s.AppendIfGeneration(key, generation, Message{Role: "user", Text: "before"})
	s.Reset(key)
	if s.AppendIfGeneration(key, generation, Message{Role: "assistant", Text: "late"}) {
		t.Fatal("pre-reset reply was appended")
	}
	if got := s.History(key); len(got) != 0 {
		t.Fatalf("history survived reset: %#v", got)
	}
}

func TestEvictionDoesNotReviveOldGeneration(t *testing.T) {
	s := NewStore(20, 1, 100)
	key := Key{ChatID: 1}
	generation := s.Generation(key)
	s.Reset(key)
	s.Generation(Key{ChatID: 2})
	if current := s.Generation(key); current == generation {
		t.Fatal("eviction reused an old generation")
	}
	if s.AppendIfGeneration(key, generation, Message{Role: "assistant", Text: "stale reply"}) {
		t.Fatal("eviction revived pre-reset work")
	}
}

func TestActiveConversationCannotBeEvicted(t *testing.T) {
	s := NewStore(20, 2, 100)
	key := Key{ChatID: 1}
	generation := s.Generation(key)
	unlock, ok := s.TryLock(key)
	if !ok {
		t.Fatal("could not lock conversation")
	}
	defer unlock()
	for id := int64(2); id < 10; id++ {
		s.Generation(Key{ChatID: id})
	}
	if s.Generation(key) != generation {
		t.Fatal("active conversation was evicted")
	}
	if _, ok := s.TryLock(key); ok {
		t.Fatal("scope lock was bypassed")
	}
	if len(s.entries) != 2 {
		t.Fatalf("scope limit exceeded: %d", len(s.entries))
	}
}
