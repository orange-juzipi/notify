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

// Notifier 钉钉通知器
type Notifier struct {
	config   Config
	template *template.Template
}

// New 创建钉钉通知器
func New(config Config, tmpl *template.Template) (*Notifier, error) {
	if config.WebhookURL == "" {
		return nil, fmt.Errorf("钉钉webhook URL不能为空")
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

// Send 发送钉钉通知
func (n *Notifier) Send(release *github.ReleaseInfo) error {
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
