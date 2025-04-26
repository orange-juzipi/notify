package dingtalk

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"text/template"
	"time"

	"github.com/orange-juzipi/notify/pkg/github"
	"github.com/orange-juzipi/notify/pkg/helper"
)

// Config 钉钉通知配置
type Config struct {
	Enabled    bool
	WebhookURL string
	Secret     string
}

// RateLimiter 钉钉消息速率限制器
type RateLimiter struct {
	mutex       sync.Mutex
	messageTime []time.Time
	// 钉钉机器人每分钟最多发送20条消息
	rateLimit    int
	windowLength time.Duration
	// 被限流后的冷却时间（10分钟）
	cooldownTime time.Duration
	// 是否处于冷却期
	inCooldown    bool
	cooldownUntil time.Time
}

// Notifier 钉钉通知器
type Notifier struct {
	config      Config
	template    *template.Template
	rateLimiter *RateLimiter
}

// New 创建钉钉通知器
func New(config Config, tmpl *template.Template) (*Notifier, error) {
	if config.WebhookURL == "" {
		return nil, fmt.Errorf("钉钉webhook URL不能为空")
	}

	// 创建速率限制器
	rateLimiter := &RateLimiter{
		messageTime:  make([]time.Time, 0),
		rateLimit:    20,               // 每分钟最多20条消息
		windowLength: time.Minute,      // 时间窗口为1分钟
		cooldownTime: 10 * time.Minute, // 冷却时间10分钟
		inCooldown:   false,
	}

	return &Notifier{
		config:      config,
		template:    tmpl,
		rateLimiter: rateLimiter,
	}, nil
}

// IsEnabled 是否启用
func (n *Notifier) IsEnabled() bool {
	return n.config.Enabled
}

// canSendMessage 检查是否可以发送消息
func (n *Notifier) canSendMessage() bool {
	n.rateLimiter.mutex.Lock()
	defer n.rateLimiter.mutex.Unlock()

	now := time.Now()

	// 检查是否在冷却期
	if n.rateLimiter.inCooldown {
		if now.Before(n.rateLimiter.cooldownUntil) {
			// 仍在冷却期内，不能发送
			return false
		}
		// 冷却期已过
		n.rateLimiter.inCooldown = false
		n.rateLimiter.messageTime = make([]time.Time, 0)
	}

	// 清理过期的消息时间记录
	validTimes := make([]time.Time, 0)
	windowStart := now.Add(-n.rateLimiter.windowLength)

	for _, t := range n.rateLimiter.messageTime {
		if t.After(windowStart) {
			validTimes = append(validTimes, t)
		}
	}
	n.rateLimiter.messageTime = validTimes

	// 检查是否超过速率限制
	if len(n.rateLimiter.messageTime) >= n.rateLimiter.rateLimit {
		// 触发冷却期
		n.rateLimiter.inCooldown = true
		n.rateLimiter.cooldownUntil = now.Add(n.rateLimiter.cooldownTime)
		return false
	}

	// 可以发送消息，记录本次时间
	n.rateLimiter.messageTime = append(n.rateLimiter.messageTime, now)
	return true
}

// Send 发送钉钉通知
func (n *Notifier) Send(release *github.ReleaseInfo) error {
	// 检查是否可以发送消息
	if !n.canSendMessage() {
		cooldownRemaining := n.rateLimiter.cooldownUntil.Sub(time.Now())
		if cooldownRemaining > 0 {
			return fmt.Errorf("钉钉消息发送频率超过限制（每分钟20条），正在冷却中，剩余时间：%v", cooldownRemaining.Round(time.Second))
		}
		return fmt.Errorf("钉钉消息发送频率超过限制（每分钟20条）")
	}

	content, err := helper.RenderTemplate(n.template, release)
	if err != nil {
		return err
	}

	title := fmt.Sprintf("仓库 %s/%s 发布新版本 %s", release.Owner, release.Repository, release.TagName)
	return n.sendMarkdown(title, content)
}

// 发送markdown消息
func (n *Notifier) sendMarkdown(title, text string) error {
	type markdownMsg struct {
		Title string `json:"title"`
		Text  string `json:"text"`
	}

	type dingMsg struct {
		Msgtype  string      `json:"msgtype"`
		Markdown markdownMsg `json:"markdown"`
	}

	msg := dingMsg{
		Msgtype: "markdown",
		Markdown: markdownMsg{
			Title: title,
			Text:  text,
		},
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %v", err)
	}

	// 添加签名
	webhook := n.config.WebhookURL
	if n.config.Secret != "" {
		webhook = n.addSignature(webhook)
	}

	resp, err := http.Post(webhook, "application/json", bytes.NewBuffer(msgBytes))
	if err != nil {
		return fmt.Errorf("发送消息失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("请求失败，状态码: %d", resp.StatusCode)
	}

	return nil
}

// 添加签名
func (n *Notifier) addSignature(webhook string) string {
	timestamp := fmt.Sprintf("%d", time.Now().UnixMilli())
	stringToSign := fmt.Sprintf("%s\n%s", timestamp, n.config.Secret)

	// 计算签名
	mac := hmac.New(sha256.New, []byte(n.config.Secret))
	mac.Write([]byte(stringToSign))
	sign := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	// 添加签名参数
	u, _ := url.Parse(webhook)
	query := u.Query()
	query.Add("timestamp", timestamp)
	query.Add("sign", sign)
	u.RawQuery = query.Encode()

	return u.String()
}
