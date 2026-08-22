package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"Pilot/internal/memorydb"
)

type MySQLStore struct {
	db      *sql.DB
	queries *memorydb.Queries
}

func NewMySQLStore(db *sql.DB) (*MySQLStore, error) {
	if db == nil {
		return nil, errors.New("mysql db is nil")
	}
	return &MySQLStore{
		db:      db,
		queries: memorydb.New(db),
	}, nil
}

func (s *MySQLStore) List(ctx context.Context, userID string, sessionID string, limit int) ([]ChatMessage, error) {
	if s == nil || s.queries == nil {
		return nil, errors.New("mysql store is not initialized")
	}
	if limit <= 0 {
		return nil, errors.New("history limit must be positive")
	}

	rows, err := s.queries.ListHistory(ctx, memorydb.ListHistoryParams{
		UserID:    userID,
		SessionID: sessionID,
		Limit:     int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list conversation history: %w", err)
	}

	messages := make([]ChatMessage, 0, len(rows))
	for _, row := range rows {
		message, err := fromDatabaseMessage(row)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func (s *MySQLStore) Append(ctx context.Context, messages []ChatMessage) error {
	if s == nil || s.db == nil {
		return errors.New("mysql store is not initialized")
	}
	if len(messages) == 0 {
		return errors.New("messages cannot be empty")
	}
	paramsList := make([]memorydb.InsertMessageParams, 0, len(messages))
	for _, message := range messages {
		params, err := toInsertParams(message)
		if err != nil {
			return err
		}
		paramsList = append(paramsList, params)
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second*3)
	defer cancel()
	// TODO: S级核心逻辑：事务中逐条调用 txQueries.InsertMessage，任一失败必须回滚。
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("failed to start tx: %w", err)
	}
	txQueries := s.queries.WithTx(tx)
	defer func() {
		//已经 commit，rollback 不会做任何操作
		_ = tx.Rollback()
	}()

	for _, params := range paramsList {
		err = txQueries.InsertMessage(ctx, params)
		if err != nil {
			// 这里会触发defer的Rollback，全部回滚
			return fmt.Errorf("insert message %q failed: %w", params.MessageID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("tx commit failed: %w", err)
	}

	return nil
}

func (s *MySQLStore) Delete(ctx context.Context, userID string, sessionID string) error {
	if s == nil || s.queries == nil {
		return errors.New("mysql store is not initialized")
	}
	if userID == "" || sessionID == "" {
		return errors.New("user id and session id are required")
	}
	if err := s.queries.DeleteHistory(ctx, memorydb.DeleteHistoryParams{
		UserID:    userID,
		SessionID: sessionID,
	}); err != nil {
		return fmt.Errorf("delete conversation history: %w", err)
	}
	return nil
}

func fromDatabaseMessage(row memorydb.ConversationHistory) (ChatMessage, error) {
	message := ChatMessage{
		ID:        row.MessageID,
		UserID:    row.UserID,
		SessionID: row.SessionID,
		Role:      Role(row.Role),
		Content:   row.Content,
		CreatedAt: row.CreatedAt,
	}

	if len(row.Metadata) > 0 {
		if err := json.Unmarshal(row.Metadata, &message.Metadata); err != nil {
			return ChatMessage{}, fmt.Errorf("decode message %q metadata: %w", row.MessageID, err)
		}
	}
	if err := message.Validate(); err != nil {
		return ChatMessage{}, fmt.Errorf("decode message %q: %w", row.MessageID, err)
	}
	return message, nil
}

func toInsertParams(message ChatMessage) (memorydb.InsertMessageParams, error) {
	if err := message.Validate(); err != nil {
		return memorydb.InsertMessageParams{}, fmt.Errorf("validate message %q: %w", message.ID, err)
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now().UTC()
	}

	metadata, err := json.Marshal(message.Metadata)
	if err != nil {
		return memorydb.InsertMessageParams{}, fmt.Errorf("marshal metadata %q: %w", message.ID, err)
	}

	return memorydb.InsertMessageParams{
		MessageID: message.ID,
		UserID:    message.UserID,
		SessionID: message.SessionID,
		Role:      string(message.Role),
		Content:   message.Content,
		Metadata:  metadata,
		CreatedAt: message.CreatedAt,
	}, nil
}
