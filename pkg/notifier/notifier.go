package notifier

import (
	"text/template"

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

	for _, release := range releases {
		for _, n := range m.notifiers {
			if n.IsEnabled() {
				if err := n.Send(release); err != nil {
					errors = append(errors, err)
				}
			}
		}
	}

	return errors
}

// RenderTemplate 渲染通知模板
func RenderTemplate(tmpl *template.Template, release *github.ReleaseInfo) (string, error) {
	return helper.RenderTemplate(tmpl, release)
}
