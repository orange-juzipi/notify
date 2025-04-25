package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"text/template"

	"github.com/orange-juzipi/notify/pkg/github"
	"github.com/orange-juzipi/notify/pkg/helper"
)

// Config Telegram通知配置
type Config struct {
	Enabled  bool
	BotToken string
	ChatID   string
}

// Notifier Telegram通知器
type Notifier struct {
	config   Config
	template *template.Template
}

// New 创建Telegram通知器
func New(config Config, tmpl *template.Template) (*Notifier, error) {
	if config.BotToken == "" {
		return nil, fmt.Errorf("Telegram Bot Token不能为空")
	}

	if config.ChatID == "" {
		return nil, fmt.Errorf("Telegram Chat ID不能为空")
	}

	return &Notifier{
		config:   config,
		template: tmpl,
	}, nil
}

// IsEnabled 是否启用
func (n *Notifier) IsEnabled() bool {
	return n.config.Enabled
}

// Send 发送Telegram通知
func (n *Notifier) Send(release *github.ReleaseInfo) error {
	content, err := helper.RenderTemplate(n.template, release)
	if err != nil {
		return err
	}

	return n.sendMessage(content)
}

// sendMessage 发送消息到Telegram
func (n *Notifier) sendMessage(text string) error {
	type messageRequest struct {
		ChatID    string `json:"chat_id"`
		Text      string `json:"text"`
		ParseMode string `json:"parse_mode"`
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.config.BotToken)

	// 准备请求参数
	msg := messageRequest{
		ChatID:    n.config.ChatID,
		Text:      text,
		ParseMode: "Markdown", // 支持Markdown格式的消息
	}

	// 将消息序列化为JSON
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %v", err)
	}

	// 发送请求
	resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(msgBytes))
	if err != nil {
		return fmt.Errorf("发送消息失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("请求失败，状态码: %d", resp.StatusCode)
	}

	// 解析响应
	var response struct {
		OK bool `json:"ok"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("解析响应失败: %v", err)
	}

	if !response.OK {
		return fmt.Errorf("Telegram API返回错误")
	}

	return nil
}
