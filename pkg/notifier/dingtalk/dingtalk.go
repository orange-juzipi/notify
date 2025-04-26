package dingtalk

import (
	"bytes"
	"context"
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

	"golang.org/x/time/rate"

	"github.com/orange-juzipi/notify/pkg/github"
)

// Config 钉钉通知配置
type Config struct {
	Enabled    bool
	WebhookURL string
	Secret     string
}

// Notifier 钉钉通知器
type Notifier struct {
	config   Config
	template *template.Template
	limiter  *rate.Limiter // 速率限制器
	client   *http.Client  // 复用HTTP客户端，提高性能
	mu       sync.Mutex    // 用于保护冷却状态
	cooldown struct {
		active bool
		until  time.Time
	}
}

// New 创建钉钉通知器
func New(config Config, tmpl *template.Template) (*Notifier, error) {
	if config.WebhookURL == "" {
		return nil, fmt.Errorf("钉钉webhook URL不能为空")
	}

	// 创建速率限制器
	// 钉钉API限制为每分钟20条消息，设置为每3秒一条，突发允许5条
	limiter := rate.NewLimiter(rate.Every(3*time.Second), 5)

	// 创建带超时的HTTP客户端
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	return &Notifier{
		config:   config,
		template: tmpl,
		limiter:  limiter,
		client:   client,
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

// Send 发送钉钉通知
func (n *Notifier) Send(release *github.ReleaseInfo) error {
	// 检查是否可以发送消息
	canSend, remaining := n.canSendMessage()
	if !canSend {
		return fmt.Errorf("钉钉消息发送频率超过限制，冷却中，剩余时间：%v", remaining.Round(time.Second))
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

	title := fmt.Sprintf("仓库 %s/%s 发布新版本 %s", release.Owner, release.Repository, release.TagName)
	err = n.sendMarkdown(title, content)

	// 检查是否需要触发冷却期
	if err != nil && (err.Error() == "频率超过限制" ||
		err.Error() == "too many requests" ||
		err.Error() == "rate limit exceeded") {
		// 触发10分钟冷却期
		n.setCooldown(10 * time.Minute)
		return fmt.Errorf("触发钉钉API限流，已设置10分钟冷却期: %v", err)
	}

	return err
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

	// 使用复用的HTTP客户端
	resp, err := n.client.Post(webhook, "application/json", bytes.NewBuffer(msgBytes))
	if err != nil {
		return fmt.Errorf("发送消息失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("请求失败，状态码: %d", resp.StatusCode)
	}

	// 解析响应，检查是否有错误
	var response struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("解析响应失败: %v", err)
	}

	if response.ErrCode != 0 {
		// 错误码88表示频率超过限制
		if response.ErrCode == 88 {
			return fmt.Errorf("频率超过限制")
		}
		return fmt.Errorf("钉钉API错误: %s (code: %d)", response.ErrMsg, response.ErrCode)
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
