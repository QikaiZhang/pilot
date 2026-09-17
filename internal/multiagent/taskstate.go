// 任务快照与断点续跑：编排的中间状态（计划、已完成分支的证据）持久化为短生命周期快照。
// 设计取舍（对应压面题 6-8 的定稿）：
//   - 快照存 Redis（短 TTL），终态主动删除、TTL 兜底——调度态是分钟级生命周期，
//     不做全量持久化；终态审计走会话记忆层，不在这里重复落库。
//   - 快照带 StartedAt 时间戳与状态机字段：只有 running 且未超窗口的快照才允许续跑，
//     失败重试读到旧状态时直接丢弃重跑，不覆盖新任务。
package multiagent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	projectagent "Pilot/internal/agent"

	"github.com/redis/go-redis/v9"
)

// TaskStatus 是编排任务的状态机字段，防止失败重试读到旧快照。
type TaskStatus string

const (
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusSucceeded TaskStatus = "succeeded"
	TaskStatusFailed    TaskStatus = "failed"
)

// DefaultSnapshotTTL 是快照的 Redis 过期时间。依据：单次编排请求级上限 30s
// （AgentPolicy.Execution.Timeout），乘安全系数后取分钟级上限 5 分钟；
// 上线后按实际任务时长分布迭代，不作为常量承诺。
const DefaultSnapshotTTL = 5 * time.Minute

const taskKeyPrefix = "pilot:multiagent:task:"

// TaskState 是一次编排的快照。Findings 只记录已成功分支的证据；
// 失败分支不进 CompletedRoles，续跑时会重新执行。
type TaskState struct {
	TaskID         string                 `json:"task_id"`
	Query          string                 `json:"query"`
	Status         TaskStatus             `json:"status"`
	StartedAt      time.Time              `json:"started_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	Plan           *PlanResult            `json:"plan,omitempty"`
	CompletedRoles []Role                 `json:"completed_roles,omitempty"`
	Findings       []projectagent.Finding `json:"findings,omitempty"`
}

// TaskStore 是任务快照的存取契约。实现必须容许并发调用。
type TaskStore interface {
	Save(ctx context.Context, state TaskState) error
	Load(ctx context.Context, taskID string) (TaskState, bool, error)
	Delete(ctx context.Context, taskID string) error
}

// RedisTaskStore 把快照存为带 TTL 的 JSON 字符串。
type RedisTaskStore struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRedisTaskStore 构造 Redis 快照存储，TTL 取 DefaultSnapshotTTL。
func NewRedisTaskStore(client *redis.Client) (*RedisTaskStore, error) {
	if client == nil {
		return nil, errors.New("redis client is nil")
	}
	return &RedisTaskStore{client: client, ttl: DefaultSnapshotTTL}, nil
}

func (s *RedisTaskStore) Save(ctx context.Context, state TaskState) error {
	if s == nil || s.client == nil {
		return errors.New("task snapshot store is not initialized")
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal task snapshot %s: %w", state.TaskID, err)
	}
	if err := s.client.Set(ctx, taskKeyPrefix+state.TaskID, data, s.ttl).Err(); err != nil {
		return fmt.Errorf("save task snapshot %s: %w", state.TaskID, err)
	}
	return nil
}

func (s *RedisTaskStore) Load(ctx context.Context, taskID string) (TaskState, bool, error) {
	if s == nil || s.client == nil {
		return TaskState{}, false, errors.New("task snapshot store is not initialized")
	}
	data, err := s.client.Get(ctx, taskKeyPrefix+taskID).Bytes()
	if errors.Is(err, redis.Nil) {
		return TaskState{}, false, nil
	}
	if err != nil {
		return TaskState{}, false, fmt.Errorf("load task snapshot %s: %w", taskID, err)
	}
	var state TaskState
	if err := json.Unmarshal(data, &state); err != nil {
		return TaskState{}, false, fmt.Errorf("decode task snapshot %s: %w", taskID, err)
	}
	return state, true, nil
}

func (s *RedisTaskStore) Delete(ctx context.Context, taskID string) error {
	if s == nil || s.client == nil {
		return errors.New("task snapshot store is not initialized")
	}
	if err := s.client.Del(ctx, taskKeyPrefix+taskID).Err(); err != nil {
		return fmt.Errorf("delete task snapshot %s: %w", taskID, err)
	}
	return nil
}

// MemoryTaskStore 是进程内快照存储：测试与无 Redis 场景的降级实现。
type MemoryTaskStore struct {
	mu     sync.Mutex
	states map[string]TaskState
}

func NewMemoryTaskStore() *MemoryTaskStore {
	return &MemoryTaskStore{states: make(map[string]TaskState)}
}

func (s *MemoryTaskStore) Save(_ context.Context, state TaskState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[state.TaskID] = state
	return nil
}

func (s *MemoryTaskStore) Load(_ context.Context, taskID string) (TaskState, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[taskID]
	return state, ok, nil
}

func (s *MemoryTaskStore) Delete(_ context.Context, taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.states, taskID)
	return nil
}

// taskIDFor 从会话与查询派生稳定的任务 ID：同一会话同一查询的重试/恢复
// 落到同一快照，不同任务天然隔离。截取 16 个十六进制字符足够防碰撞且键名短。
func taskIDFor(request projectagent.AgentRequest) string {
	digest := sha256.Sum256([]byte(request.SessionID + "\x00" + request.Query))
	return hex.EncodeToString(digest[:])[:16]
}
