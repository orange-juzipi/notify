package notifier

import (
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/orange-juzipi/notify/config"
	"github.com/orange-juzipi/notify/pkg/github"
	"github.com/orange-juzipi/notify/pkg/helper"
	"github.com/orange-juzipi/notify/pkg/notifier/dingtalk"
	"github.com/orange-juzipi/notify/pkg/notifier/telegram"
)

// Notifier 通知器接口
type Notifier interface {
	// Send 发送通知
	Send(release *github.ReleaseInfo) error
	// IsEnabled 是否启用
	IsEnabled() bool
}

// Manager 通知管理器
type Manager struct {
	notifiers []Notifier
	template  *template.Template
}

// NewManager 创建通知管理器
func NewManager(cfg *config.Config) (*Manager, error) {
	// 解析模板
	tmpl, err := template.New("release").Parse(cfg.Template)
	if err != nil {
		return nil, err
	}

	// 创建通知器
	manager := &Manager{
		template: tmpl,
	}

	// 添加钉钉通知器
	if cfg.Notifications.DingTalk.Enabled {
		dingTalkConfig := dingtalk.Config{
			Enabled:    cfg.Notifications.DingTalk.Enabled,
			WebhookURL: cfg.Notifications.DingTalk.WebhookURL,
			Secret:     cfg.Notifications.DingTalk.Secret,
		}
		dingTalk, err := dingtalk.New(dingTalkConfig, tmpl)
		if err != nil {
			return nil, err
		}
		manager.notifiers = append(manager.notifiers, dingTalk)
	}

	// 添加Telegram通知器
	if cfg.Notifications.Telegram.Enabled {
		telegramConfig := telegram.Config{
			Enabled:  cfg.Notifications.Telegram.Enabled,
			BotToken: cfg.Notifications.Telegram.BotToken,
			ChatID:   cfg.Notifications.Telegram.ChatID,
		}
		telegram, err := telegram.New(telegramConfig, tmpl)
		if err != nil {
			return nil, err
		}
		manager.notifiers = append(manager.notifiers, telegram)
	}

	return manager, nil
}

// NotifyAll 向所有启用的通知器发送通知
func (m *Manager) NotifyAll(releases []*github.ReleaseInfo) []error {
	var errors []error

	// 为了确保消息不会超过钉钉的频率限制（每分钟20条），我们会分批发送
	const batchSize = 15                   // 设置小于限制的安全值
	const batchInterval = 60 * time.Second // 每批次之间的时间间隔

	// 按批次处理发布信息
	for i := 0; i < len(releases); i += batchSize {
		end := i + batchSize
		if end > len(releases) {
			end = len(releases)
		}

		batch := releases[i:end]
		batchErrors := m.sendBatch(batch)
		errors = append(errors, batchErrors...)

		// 如果还有更多批次需要处理，等待一段时间
		if end < len(releases) {
			fmt.Printf("已发送 %d/%d 条通知，为避免超过频率限制，等待 %v 后继续...\n",
				end, len(releases), batchInterval)
			time.Sleep(batchInterval)
		}
	}

	return errors
}

// sendBatch 发送一批通知
func (m *Manager) sendBatch(releases []*github.ReleaseInfo) []error {
	var errors []error

	for _, release := range releases {
		for _, n := range m.notifiers {
			if n.IsEnabled() {
				if err := n.Send(release); err != nil {
					// 检查是否是速率限制错误
					if isRateLimitError(err) {
						fmt.Printf("警告: 遇到速率限制 - %v\n", err)
						// 对于速率限制错误，我们添加一个特殊的错误消息
						errors = append(errors, fmt.Errorf("速率限制触发: %v", err))
						// 不再继续发送其他通知，避免冷却期延长
						return errors
					}
					errors = append(errors, err)
				}
				// 每次发送后稍微等待一下，进一步降低触发限流的风险
				time.Sleep(100 * time.Millisecond)
			}
		}
	}

	return errors
}

// isRateLimitError 判断是否是速率限制错误
func isRateLimitError(err error) bool {
	errMsg := err.Error()
	return strings.Contains(errMsg, "频率超过限制") ||
		strings.Contains(errMsg, "rate limit") ||
		strings.Contains(errMsg, "too many requests")
}

// RenderTemplate 渲染通知模板
func RenderTemplate(tmpl *template.Template, release *github.ReleaseInfo) (string, error) {
	return helper.RenderTemplate(tmpl, release)
}
