package notifier

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"
	"text/template"
	"time"

	"golang.org/x/time/rate"

	"github.com/orange-juzipi/notify/config"
	"github.com/orange-juzipi/notify/pkg/github"
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
	limiter   *rate.Limiter
}

// NewManager 创建通知管理器
func NewManager(cfg *config.Config) (*Manager, error) {
	// 解析模板
	tmpl, err := template.New("release").Parse(cfg.Template)
	if err != nil {
		return nil, err
	}

	// 使用标准库的速率限制器，每分钟20条消息
	// 设置为 1条/3秒，突发容量为5条，更加安全
	limiter := rate.NewLimiter(rate.Every(3*time.Second), 5)

	// 创建通知器
	manager := &Manager{
		template: tmpl,
		limiter:  limiter,
	}

	// 添加钉钉通知器
	if cfg.Notifications.DingTalk.Enabled {
		dingTalkConfig := dingtalk.Config{
			Enabled:    cfg.Notifications.DingTalk.Enabled,
			WebhookURL: cfg.Notifications.DingTalk.WebhookURL,
			Secret:     cfg.Notifications.DingTalk.Secret,
		}
		err = manager.AddDingTalkNotifier(dingTalkConfig)
		if err != nil {
			return nil, err
		}
	}

	// 添加Telegram通知器
	if cfg.Notifications.Telegram.Enabled {
		telegramConfig := telegram.Config{
			Enabled:  cfg.Notifications.Telegram.Enabled,
			BotToken: cfg.Notifications.Telegram.BotToken,
			ChatID:   cfg.Notifications.Telegram.ChatID,
		}
		err = manager.AddTelegramNotifier(telegramConfig)
		if err != nil {
			return nil, err
		}
	}

	return manager, nil
}

// NotifyAll 向所有启用的通知器发送通知
func (m *Manager) NotifyAll(releases []*github.ReleaseInfo) []error {
	var errors []error
	ctx := context.Background()

	// 以最大限流速率作为批次处理周期
	const batchSize = 10
	const batchTimeout = 30 * time.Second

	log.Printf("开始发送通知，共 %d 条...", len(releases))

	// 按批次处理发布信息
	for i := 0; i < len(releases); i += batchSize {
		end := i + batchSize
		if end > len(releases) {
			end = len(releases)
		}

		batch := releases[i:end]
		batchCtx, cancel := context.WithTimeout(ctx, batchTimeout)
		batchErrors := m.sendBatch(batchCtx, batch)
		cancel()

		errors = append(errors, batchErrors...)

		// 如果还有更多批次，等待一下再继续
		if end < len(releases) {
			// 打印当前进度
			log.Printf("已发送 %d/%d 条通知 (%d%%)", end, len(releases), end*100/len(releases))
			time.Sleep(2 * time.Second)
		}
	}

	return errors
}

// sendBatch 发送一批通知
func (m *Manager) sendBatch(ctx context.Context, releases []*github.ReleaseInfo) []error {
	var errors []error

	for _, release := range releases {
		for _, n := range m.notifiers {
			if !n.IsEnabled() {
				continue
			}

			// 使用标准库限流器等待令牌
			err := m.limiter.Wait(ctx)
			if err != nil {
				errors = append(errors, fmt.Errorf("限流等待错误: %v", err))
				continue
			}

			// 发送通知
			if err := n.Send(release); err != nil {
				// 检查是否是速率限制错误
				if isRateLimitError(err) {
					log.Printf("警告: 遇到速率限制 - %v", err)
					// 对于速率限制错误，等待较长时间后再尝试
					time.Sleep(5 * time.Second)
					errors = append(errors, fmt.Errorf("速率限制: %v", err))
				} else {
					errors = append(errors, err)
				}
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
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, release); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// AddDingTalkNotifier 添加钉钉通知器
func (m *Manager) AddDingTalkNotifier(config dingtalk.Config) error {
	if !config.Enabled {
		return nil
	}

	notifier, err := dingtalk.New(config, m.template)
	if err != nil {
		return err
	}

	m.notifiers = append(m.notifiers, notifier)
	return nil
}

// AddTelegramNotifier 添加Telegram通知器
func (m *Manager) AddTelegramNotifier(config telegram.Config) error {
	if !config.Enabled {
		return nil
	}

	notifier, err := telegram.New(config, m.template)
	if err != nil {
		return err
	}

	m.notifiers = append(m.notifiers, notifier)
	return nil
}
