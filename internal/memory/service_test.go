package memory

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeHistoryStore struct {
	messages     []ChatMessage
	listErr      error
	appendErr    error
	listCalls    int
	appendCalls  int
	appendedData []ChatMessage
}

func (f *fakeHistoryStore) Append(_ context.Context, messages []ChatMessage) error {
	f.appendCalls++
	f.appendedData = messages
	return f.appendErr
}

func (f *fakeHistoryStore) List(_ context.Context, userID, sessionID string, limit int) ([]ChatMessage, error) {
	f.listCalls++
	if f.listErr != nil {
		return nil, f.listErr
	}
	if len(f.messages) > limit {
		return f.messages[:limit], nil
	}
	return f.messages, nil
}

func (f *fakeHistoryStore) Delete(context.Context, string, string) error { return nil }

type fakeRecentMemory struct {
	messages     []ChatMessage
	getErr       error
	appendErr    error
	getCalls     int
	appendCalls  int
	appendLimit  int
	appendTTL    time.Duration
	appendedData []ChatMessage
}

func (f *fakeRecentMemory) Get(context.Context, string, string, int) ([]ChatMessage, error) {
	f.getCalls++
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.messages, nil
}

func (f *fakeRecentMemory) Append(_ context.Context, messages []ChatMessage, limit int, ttl time.Duration) error {
	f.appendCalls++
	f.appendLimit = limit
	f.appendTTL = ttl
	f.appendedData = messages
	return f.appendErr
}

func (f *fakeRecentMemory) Clear(context.Context, string, string) error { return nil }

func testService(t *testing.T, history *fakeHistoryStore, recent *fakeRecentMemory) *Service {
	t.Helper()
	service, err := NewService(history, recent, ServiceConfig{
		RecentLimit: 10,
		RecentTTL:   time.Hour,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func testMessage() ChatMessage {
	return ChatMessage{
		ID:        "message-1",
		UserID:    "user-1",
		SessionID: "session-1",
		Role:      RoleUser,
		Content:   "hello",
	}
}

func TestServiceLoadRecentReturnsCacheHit(t *testing.T) {
	cached := testMessage()
	history := &fakeHistoryStore{messages: []ChatMessage{testMessage()}}
	recent := &fakeRecentMemory{messages: []ChatMessage{cached}}

	got, err := testService(t, history, recent).LoadRecent(context.Background(), "user-1", "session-1", 5)
	if err != nil {
		t.Fatalf("LoadRecent() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != cached.ID {
		t.Fatalf("LoadRecent() = %+v, want cache hit", got)
	}
	if history.listCalls != 0 {
		t.Fatalf("history.List() calls = %d, want 0", history.listCalls)
	}
}

func TestServiceLoadRecentFallsBackAndRefreshesCache(t *testing.T) {
	historyMessage := testMessage()
	history := &fakeHistoryStore{messages: []ChatMessage{historyMessage}}
	recent := &fakeRecentMemory{}

	got, err := testService(t, history, recent).LoadRecent(context.Background(), "user-1", "session-1", 5)
	if err != nil {
		t.Fatalf("LoadRecent() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != historyMessage.ID {
		t.Fatalf("LoadRecent() = %+v, want history result", got)
	}
	if history.listCalls != 1 || recent.appendCalls != 1 {
		t.Fatalf("calls = history.List:%d recent.Append:%d, want 1 and 1", history.listCalls, recent.appendCalls)
	}
	if recent.appendLimit != 5 || recent.appendTTL != time.Hour {
		t.Fatalf("refresh arguments = limit %d ttl %v", recent.appendLimit, recent.appendTTL)
	}
}

func TestServiceLoadRecentReturnsEmptyHistory(t *testing.T) {
	history := &fakeHistoryStore{}
	recent := &fakeRecentMemory{}

	got, err := testService(t, history, recent).LoadRecent(context.Background(), "user-1", "session-1", 5)
	if err != nil {
		t.Fatalf("LoadRecent() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("LoadRecent() = %+v, want empty result", got)
	}
	if recent.appendCalls != 0 {
		t.Fatalf("recent.Append() calls = %d, want 0", recent.appendCalls)
	}
}

func TestServiceLoadRecentKeepsHistoryWhenCacheRefreshFails(t *testing.T) {
	historyMessage := testMessage()
	history := &fakeHistoryStore{messages: []ChatMessage{historyMessage}}
	recent := &fakeRecentMemory{appendErr: errors.New("redis unavailable")}

	got, err := testService(t, history, recent).LoadRecent(context.Background(), "user-1", "session-1", 5)
	if err != nil {
		t.Fatalf("LoadRecent() error = %v, want nil", err)
	}
	if len(got) != 1 || got[0].ID != historyMessage.ID {
		t.Fatalf("LoadRecent() = %+v, want history result", got)
	}
}

func TestServiceLoadRecentPropagatesStoreErrors(t *testing.T) {
	storeErr := errors.New("redis unavailable")
	service := testService(t, &fakeHistoryStore{}, &fakeRecentMemory{getErr: storeErr})

	_, err := service.LoadRecent(context.Background(), "user-1", "session-1", 5)
	if !errors.Is(err, storeErr) {
		t.Fatalf("LoadRecent() error = %v, want wrapped %v", err, storeErr)
	}
}

func TestServiceLoadRecentPropagatesHistoryErrors(t *testing.T) {
	storeErr := errors.New("mysql unavailable")
	service := testService(t, &fakeHistoryStore{listErr: storeErr}, &fakeRecentMemory{})

	_, err := service.LoadRecent(context.Background(), "user-1", "session-1", 5)
	if !errors.Is(err, storeErr) {
		t.Fatalf("LoadRecent() error = %v, want wrapped %v", err, storeErr)
	}
}

func TestServiceSaveMessagesStopsWhenHistoryFails(t *testing.T) {
	historyErr := errors.New("mysql unavailable")
	history := &fakeHistoryStore{appendErr: historyErr}
	recent := &fakeRecentMemory{}

	err := testService(t, history, recent).SaveMessages(context.Background(), []ChatMessage{testMessage()})
	if !errors.Is(err, historyErr) {
		t.Fatalf("SaveMessages() error = %v, want wrapped %v", err, historyErr)
	}
	if recent.appendCalls != 0 {
		t.Fatalf("recent.Append() calls = %d, want 0", recent.appendCalls)
	}
}

func TestServiceSaveMessagesWritesHistoryThenRecentMemory(t *testing.T) {
	history := &fakeHistoryStore{}
	recent := &fakeRecentMemory{}
	messages := []ChatMessage{testMessage()}

	if err := testService(t, history, recent).SaveMessages(context.Background(), messages); err != nil {
		t.Fatalf("SaveMessages() error = %v", err)
	}
	if history.appendCalls != 1 || recent.appendCalls != 1 {
		t.Fatalf("calls = history.Append:%d recent.Append:%d, want 1 and 1", history.appendCalls, recent.appendCalls)
	}
	if recent.appendLimit != 10 || recent.appendTTL != time.Hour {
		t.Fatalf("recent.Append() args = limit %d ttl %v", recent.appendLimit, recent.appendTTL)
	}
}

func TestServiceSaveMessagesKeepsSuccessWhenRecentMemoryFails(t *testing.T) {
	history := &fakeHistoryStore{}
	recent := &fakeRecentMemory{appendErr: errors.New("redis unavailable")}

	if err := testService(t, history, recent).SaveMessages(context.Background(), []ChatMessage{testMessage()}); err != nil {
		t.Fatalf("SaveMessages() error = %v, want nil", err)
	}
	if history.appendCalls != 1 {
		t.Fatalf("history.Append() calls = %d, want 1", history.appendCalls)
	}
}
