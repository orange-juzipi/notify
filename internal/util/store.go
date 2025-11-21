package util

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ReleaseState 表示仓库版本的状态
type ReleaseState struct {
	Owner        string    `json:"owner"`
	Repository   string    `json:"repository"`
	LatestTag    string    `json:"latest_tag"`
	LastNotified time.Time `json:"last_notified"`
}

// StateStore 管理已处理的版本状态
type StateStore struct {
	storePath string
	states    map[string]ReleaseState
	mu        sync.RWMutex
}

// NewStateStore 创建新的状态存储
func NewStateStore(storePath string) (*StateStore, error) {
	if storePath == "" {
		// 如果没有指定路径，使用默认路径
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("获取用户主目录失败: %v", err)
		}
		storePath = filepath.Join(home, ".notify", "state.json")
	}

	// 确保目录存在
	storeDir := filepath.Dir(storePath)
	if err := os.MkdirAll(storeDir, 0755); err != nil {
		return nil, fmt.Errorf("创建状态存储目录失败: %v", err)
	}

	store := &StateStore{
		storePath: storePath,
		states:    make(map[string]ReleaseState),
	}

	// 尝试加载现有状态
	if _, err := os.Stat(storePath); err == nil {
		if err := store.load(); err != nil {
			return nil, fmt.Errorf("加载状态文件失败: %v", err)
		}
	}

	return store, nil
}

// 加载状态文件
func (s *StateStore) load() error {
	data, err := os.ReadFile(s.storePath)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return json.Unmarshal(data, &s.states)
}

// 保存状态文件
func (s *StateStore) save() error {
	s.mu.RLock()
	data, err := json.MarshalIndent(s.states, "", "  ")
	s.mu.RUnlock()

	if err != nil {
		return err
	}

	return os.WriteFile(s.storePath, data, 0644)
}

// getKey 生成仓库的唯一键
func getKey(owner, repo string) string {
	return fmt.Sprintf("%s/%s", owner, repo)
}

// GetLatestTag 获取仓库的最新标签
func (s *StateStore) GetLatestTag(owner, repo string) string {
	key := getKey(owner, repo)

	s.mu.RLock()
	defer s.mu.RUnlock()

	if state, exists := s.states[key]; exists {
		return state.LatestTag
	}
	return ""
}

// UpdateState 更新仓库的状态
func (s *StateStore) UpdateState(owner, repo, tag string) error {
	key := getKey(owner, repo)

	s.mu.Lock()
	s.states[key] = ReleaseState{
		Owner:        owner,
		Repository:   repo,
		LatestTag:    tag,
		LastNotified: time.Now(),
	}
	s.mu.Unlock()

	return s.save()
}

// IsNewRelease 检查是否为新版本
func (s *StateStore) IsNewRelease(owner, repo, tag string) bool {
	currentTag := s.GetLatestTag(owner, repo)
	return currentTag == "" || currentTag != tag
}

// CheckAndUpdateIfNew 原子性地检查是否为新版本，如果是则更新状态并保存到文件
// 返回值: (isNew bool, err error)
// - isNew: true 表示是新版本并已更新状态，false 表示不是新版本
// - err: 更新或保存状态时的错误（如果有）
func (s *StateStore) CheckAndUpdateIfNew(owner, repo, tag string) (bool, error) {
	key := getKey(owner, repo)

	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查是否为新版本
	currentState, exists := s.states[key]

	// 如果不是新版本，直接返回
	if exists && currentState.LatestTag == tag {
		return false, nil
	}

	// 是新版本，更新内存状态
	s.states[key] = ReleaseState{
		Owner:        owner,
		Repository:   repo,
		LatestTag:    tag,
		LastNotified: time.Now(),
	}

	// 立即保存到文件（在锁内完成，确保原子性）
	// 注意：这里直接序列化和写文件，不使用 save() 方法，避免重复加锁
	data, err := json.MarshalIndent(s.states, "", "  ")
	if err != nil {
		// 序列化失败是严重错误，返回错误并记录
		fmt.Printf("错误: 序列化状态失败: %v\n", err)
		return false, fmt.Errorf("序列化状态失败: %v", err)
	}

	// 写入文件
	if err := os.WriteFile(s.storePath, data, 0644); err != nil {
		// 文件写入失败是严重错误
		// 但因为内存状态已更新，为了避免重复通知，我们返回 true
		// 同时记录错误日志，方便排查
		fmt.Printf("警告: 保存状态文件失败: %v (内存状态已更新，本次不会重复通知，但重启后可能重复)\n", err)
		return true, nil // 返回 true，避免本次运行重复通知
	}

	// 成功保存
	return true, nil
}

// SaveState 显式保存状态到文件
// 这个方法应该在 CheckAndUpdateIfNew 返回 true 后调用
func (s *StateStore) SaveState() error {
	return s.save()
}
