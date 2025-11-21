package util

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestCheckAndUpdateIfNew_Atomicity 测试并发场景下的原子性
func TestCheckAndUpdateIfNew_Atomicity(t *testing.T) {
	// 创建临时测试文件
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "test_state.json")

	store, err := NewStateStore(storePath)
	if err != nil {
		t.Fatalf("创建 StateStore 失败: %v", err)
	}

	const (
		owner      = "test-owner"
		repo       = "test-repo"
		tag        = "v1.0.0"
		goroutines = 100 // 并发数
	)

	// 用于统计有多少个 goroutine 认为这是新版本
	var (
		newCount int
		mu       sync.Mutex
		wg       sync.WaitGroup
	)

	// 启动多个 goroutine 并发检查和更新
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			isNew, err := store.CheckAndUpdateIfNew(owner, repo, tag)
			if err != nil {
				t.Errorf("CheckAndUpdateIfNew 失败: %v", err)
				return
			}
			if isNew {
				mu.Lock()
				newCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// 验证结果：应该只有一个 goroutine 认为这是新版本
	if newCount != 1 {
		t.Errorf("期望只有 1 个 goroutine 认为是新版本，但实际有 %d 个", newCount)
	}

	// 验证状态已正确保存
	savedTag := store.GetLatestTag(owner, repo)
	if savedTag != tag {
		t.Errorf("期望保存的标签是 %s，但实际是 %s", tag, savedTag)
	}
}

// TestCheckAndUpdateIfNew_DifferentTags 测试不同标签的更新
func TestCheckAndUpdateIfNew_DifferentTags(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "test_state.json")

	store, err := NewStateStore(storePath)
	if err != nil {
		t.Fatalf("创建 StateStore 失败: %v", err)
	}

	const (
		owner = "test-owner"
		repo  = "test-repo"
	)

	// 第一次更新
	isNew, err := store.CheckAndUpdateIfNew(owner, repo, "v1.0.0")
	if err != nil {
		t.Fatalf("第一次 CheckAndUpdateIfNew 失败: %v", err)
	}
	if !isNew {
		t.Error("第一次应该是新版本")
	}

	// 第二次使用相同标签
	isNew, err = store.CheckAndUpdateIfNew(owner, repo, "v1.0.0")
	if err != nil {
		t.Fatalf("第二次 CheckAndUpdateIfNew 失败: %v", err)
	}
	if isNew {
		t.Error("相同标签不应该被认为是新版本")
	}

	// 第三次使用不同标签
	isNew, err = store.CheckAndUpdateIfNew(owner, repo, "v2.0.0")
	if err != nil {
		t.Fatalf("第三次 CheckAndUpdateIfNew 失败: %v", err)
	}
	if !isNew {
		t.Error("不同标签应该被认为是新版本")
	}

	// 验证最终状态
	savedTag := store.GetLatestTag(owner, repo)
	if savedTag != "v2.0.0" {
		t.Errorf("期望保存的标签是 v2.0.0，但实际是 %s", savedTag)
	}
}

// TestCheckAndUpdateIfNew_SaveState 测试保存状态到文件
func TestCheckAndUpdateIfNew_SaveState(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "test_state.json")

	store, err := NewStateStore(storePath)
	if err != nil {
		t.Fatalf("创建 StateStore 失败: %v", err)
	}

	const (
		owner = "test-owner"
		repo  = "test-repo"
		tag   = "v1.0.0"
	)

	// 检查并更新
	isNew, err := store.CheckAndUpdateIfNew(owner, repo, tag)
	if err != nil {
		t.Fatalf("CheckAndUpdateIfNew 失败: %v", err)
	}
	if !isNew {
		t.Error("应该是新版本")
	}

	// 保存状态
	if err := store.SaveState(); err != nil {
		t.Fatalf("SaveState 失败: %v", err)
	}

	// 验证文件是否存在
	if _, err := os.Stat(storePath); os.IsNotExist(err) {
		t.Error("状态文件未创建")
	}

	// 创建新的 store 实例，验证能否从文件加载
	newStore, err := NewStateStore(storePath)
	if err != nil {
		t.Fatalf("从文件加载 StateStore 失败: %v", err)
	}

	// 验证加载的状态
	savedTag := newStore.GetLatestTag(owner, repo)
	if savedTag != tag {
		t.Errorf("期望加载的标签是 %s，但实际是 %s", tag, savedTag)
	}
}

// BenchmarkCheckAndUpdateIfNew 性能基准测试
func BenchmarkCheckAndUpdateIfNew(b *testing.B) {
	tmpDir := b.TempDir()
	storePath := filepath.Join(tmpDir, "bench_state.json")

	store, err := NewStateStore(storePath)
	if err != nil {
		b.Fatalf("创建 StateStore 失败: %v", err)
	}

	const (
		owner = "bench-owner"
		repo  = "bench-repo"
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tag := time.Now().String() // 每次使用不同的标签
		_, err := store.CheckAndUpdateIfNew(owner, repo, tag)
		if err != nil {
			b.Fatalf("CheckAndUpdateIfNew 失败: %v", err)
		}
	}
}
