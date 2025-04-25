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
