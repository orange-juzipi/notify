package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"text/template"
	"time"

	"golang.org/x/time/rate"

	"github.com/orange-juzipi/notify/pkg/github"
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
	client   *http.Client
	limiter  *rate.Limiter // 速率限制器
	mu       sync.Mutex    // 保护冷却状态
	cooldown struct {
		active bool
		until  time.Time
	}
}

// New 创建Telegram通知器
func New(config Config, tmpl *template.Template) (*Notifier, error) {
	if config.BotToken == "" {
		return nil, fmt.Errorf("Telegram Bot Token不能为空")
	}

	if config.ChatID == "" {
		return nil, fmt.Errorf("Telegram Chat ID不能为空")
	}

	// 速率限制器
	// Telegram API限制: 每秒1条消息
	limiter := rate.NewLimiter(rate.Every(1*time.Second), 3)

	// 创建带超时的HTTP客户端
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	return &Notifier{
		config:   config,
		template: tmpl,
		client:   client,
		limiter:  limiter,
		cooldown: struct {
			active bool
			until  time.Time
		}{
			active: false,
		},
	}, nil
}

// IsEnabled 是否启用
func (n *Notifier) IsEnabled() bool {
	return n.config.Enabled
}

// renderTemplate 渲染通知模板
func (n *Notifier) renderTemplate(release *github.ReleaseInfo) (string, error) {
	var buf bytes.Buffer
	if err := n.template.Execute(&buf, release); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// canSendMessage 检查是否可以发送消息
func (n *Notifier) canSendMessage() (bool, time.Duration) {
	n.mu.Lock()
	defer n.mu.Unlock()

	now := time.Now()

	// 检查是否在冷却期
	if n.cooldown.active && now.Before(n.cooldown.until) {
		return false, n.cooldown.until.Sub(now)
	}

	// 冷却期已过或未激活
	n.cooldown.active = false
	return true, 0
}

// setCooldown 设置冷却期
func (n *Notifier) setCooldown(duration time.Duration) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.cooldown.active = true
	n.cooldown.until = time.Now().Add(duration)
}

// Send 发送Telegram通知
func (n *Notifier) Send(release *github.ReleaseInfo) error {
	// 检查是否可以发送消息
	canSend, remaining := n.canSendMessage()
	if !canSend {
		return fmt.Errorf("Telegram消息发送频率超过限制，冷却中，剩余时间：%v", remaining.Round(time.Second))
	}

	// 控制发送频率
	ctx := context.Background()
	if err := n.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("速率限制等待错误: %v", err)
	}

	content, err := n.renderTemplate(release)
	if err != nil {
		return err
	}

	err = n.sendMessage(content)
	if err != nil && (err.Error() == "too many requests" || err.Error() == "rate limit exceeded") {
		// Telegram 429 错误触发冷却期
		n.setCooldown(1 * time.Minute)
		return fmt.Errorf("触发Telegram API限流，已设置1分钟冷却期: %v", err)
	}

	return err
}

// SendBatch 批量发送Telegram通知（合并成一条消息）
func (n *Notifier) SendBatch(releases []*github.ReleaseInfo) error {
	if len(releases) == 0 {
		return nil
	}

	// 检查是否可以发送消息
	canSend, remaining := n.canSendMessage()
	if !canSend {
		return fmt.Errorf("Telegram消息发送频率超过限制，冷却中，剩余时间：%v", remaining.Round(time.Second))
	}

	// 控制发送频率
	ctx := context.Background()
	if err := n.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("速率限制等待错误: %v", err)
	}

	// 构建批量消息内容
	var content bytes.Buffer
	content.WriteString("📦 *GitHub 版本更新汇总*\n\n")
	content.WriteString(fmt.Sprintf("共 %d 个仓库发布了新版本：\n\n", len(releases)))

	for i, release := range releases {
		content.WriteString(fmt.Sprintf("*%d. %s/%s*\n",
			i+1, release.Owner, release.Repository))
		content.WriteString(fmt.Sprintf("版本: `%s`\n", release.TagName))
		content.WriteString(fmt.Sprintf("时间: %s\n",
			release.PublishedAt.Format("2006-01-02 15:04:05")))
		content.WriteString(fmt.Sprintf("[查看详情](%s)\n\n", release.HTMLURL))
	}

	err := n.sendMessage(content.String())
	if err != nil && (err.Error() == "too many requests" || err.Error() == "rate limit exceeded") {
		// Telegram 429 错误触发冷却期
		n.setCooldown(1 * time.Minute)
		return fmt.Errorf("触发Telegram API限流，已设置1分钟冷却期: %v", err)
	}

	return err
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

	// 发送请求，使用复用的HTTP客户端
	resp, err := n.client.Post(apiURL, "application/json", bytes.NewBuffer(msgBytes))
	if err != nil {
		return fmt.Errorf("发送消息失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应
	if resp.StatusCode == 429 {
		// HTTP 429 Too Many Requests
		return fmt.Errorf("too many requests")
	} else if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("请求失败，状态码: %d", resp.StatusCode)
	}

	// 解析响应
	var response struct {
		OK          bool   `json:"ok"`
		Description string `json:"description,omitempty"`
		ErrorCode   int    `json:"error_code,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("解析响应失败: %v", err)
	}

	if !response.OK {
		if response.ErrorCode == 429 {
			return fmt.Errorf("too many requests")
		}
		return fmt.Errorf("Telegram API返回错误: %s (code: %d)", response.Description, response.ErrorCode)
	}

	return nil
}
