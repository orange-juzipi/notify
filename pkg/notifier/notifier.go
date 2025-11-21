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
	// SendBatch 批量发送通知（合并成一条消息）
	SendBatch(releases []*github.ReleaseInfo) error
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

	// 使用标准库的速率限制器
	// 设置为 1条/4秒（15条/分钟），突发容量为3条
	// 这样配合钉钉的限制器，确保不会超过每分钟20条的硬性限制
	limiter := rate.NewLimiter(rate.Every(4*time.Second), 3)

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
// 每10个仓库合并成一条消息发送
func (m *Manager) NotifyAll(releases []*github.ReleaseInfo) []error {
	var errors []error
	ctx := context.Background()

	// 每条消息包含10个仓库的更新
	const releasesPerMessage = 10
	// 钉钉限制：每分钟最多20条消息
	// 每批发送15条消息（包含150个仓库的更新），然后等待1分钟
	const messagesPerBatch = 15
	const batchTimeout = 2 * time.Minute
	const waitBetweenBatches = 65 * time.Second

	// 计算需要发送的消息数量
	totalMessages := (len(releases) + releasesPerMessage - 1) / releasesPerMessage
	log.Printf("开始发送通知: %d 个仓库更新，合并为 %d 条消息", len(releases), totalMessages)

	messagesSent := 0

	// 按每10个仓库一组进行分组
	for i := 0; i < len(releases); i += releasesPerMessage {
		end := i + releasesPerMessage
		if end > len(releases) {
			end = len(releases)
		}

		group := releases[i:end]
		messagesSent++

		batchCtx, cancel := context.WithTimeout(ctx, batchTimeout)
		batchErrors := m.sendBatchMessage(batchCtx, group)
		cancel()

		errors = append(errors, batchErrors...)

		// 每发送 messagesPerBatch 条消息后，等待一段时间
		if messagesSent%messagesPerBatch == 0 && messagesSent < totalMessages {
			log.Printf("已发送 %d/%d 条消息，等待后继续...", messagesSent, totalMessages)
			time.Sleep(waitBetweenBatches)
		} else if messagesSent < totalMessages {
			// 消息之间的间隔（避免过快）
			time.Sleep(4 * time.Second)
		}
	}

	return errors
}

// sendBatchMessage 发送一条合并消息（包含多个仓库更新）
func (m *Manager) sendBatchMessage(ctx context.Context, releases []*github.ReleaseInfo) []error {
	var errors []error

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

		// 发送批量通知
		if err := n.SendBatch(releases); err != nil {
			// 检查是否是速率限制错误
			if isRateLimitError(err) {
				log.Printf("警告: 遇到速率限制 - %v", err)
				time.Sleep(5 * time.Second)
				errors = append(errors, fmt.Errorf("速率限制: %v", err))
			} else {
				log.Printf("发送失败 - %v", err)
				errors = append(errors, err)
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
