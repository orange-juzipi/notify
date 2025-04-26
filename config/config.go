package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config 应用配置结构
type Config struct {
	GitHub        GitHubConfig        `mapstructure:"github"`
	Notifications NotificationsConfig `mapstructure:"notifications"`
	Template      string              `mapstructure:"template"`
	Schedule      ScheduleConfig      `mapstructure:"schedule"`
}

// GitHubConfig GitHub相关配置
type GitHubConfig struct {
	Token string       `mapstructure:"token"`
	Repos []RepoConfig `mapstructure:"repos"`
	// 设置为true时，自动监控授权用户的所有仓库
	AutoWatchUser bool `mapstructure:"auto_watch_user"`
	// 设置为true时，监控用户已star的仓库
	WatchStarred bool `mapstructure:"watch_starred"`
	// 要监控的组织，如果为空则不监控组织仓库
	WatchOrgs []string `mapstructure:"watch_orgs"`
	// 设置为true时，检查仓库是否有release并只监控有release的仓库
	OnlyWithReleases bool `mapstructure:"only_with_releases"`
	// 检查最近多少天内的版本发布，默认为3天
	CheckDays int `mapstructure:"check_days"`
}

// RepoConfig 仓库配置
type RepoConfig struct {
	Owner string `mapstructure:"owner"`
	Name  string `mapstructure:"name"`
}

// NotificationsConfig 通知渠道配置
type NotificationsConfig struct {
	DingTalk DingTalkConfig `mapstructure:"dingtalk"`
	Telegram TelegramConfig `mapstructure:"telegram"`
}

// DingTalkConfig 钉钉机器人配置
type DingTalkConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	WebhookURL string `mapstructure:"webhook_url"`
	Secret     string `mapstructure:"secret"`
}

// TelegramConfig Telegram机器人配置
type TelegramConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	BotToken string `mapstructure:"bot_token"`
	ChatID   string `mapstructure:"chat_id"`
}

// ScheduleConfig 定时运行配置
type ScheduleConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Interval string `mapstructure:"interval"`
}

// DefaultTemplate 默认通知模板
const DefaultTemplate = `## 📦 新版本发布通知

**仓库**: {{.Repository}}

**版本**: {{.TagName}}

**发布时间**: {{.PublishedAt.Format "2006-01-02 15:04:05"}}

{{.Description}}

**[查看详情]({{.HTMLURL}})**`

// DefaultInterval 默认检查间隔时间 (6小时)
const DefaultInterval = "6h"

// DefaultCheckDays 默认检查最近多少天内的版本发布（3天）
const DefaultCheckDays = 3

// LoadConfig 从文件加载配置
func LoadConfig(cfgFile string) (*Config, error) {
	cfg := &Config{}

	// 如果配置文件路径为空，使用默认路径
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		// 搜索配置文件路径
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("获取用户主目录失败: %v", err)
		}

		// 在工作目录和用户主目录下查找配置文件
		viper.AddConfigPath(".")
		viper.AddConfigPath(filepath.Join(home, ".notify"))
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}

	// 读取环境变量
	viper.AutomaticEnv()
	viper.SetEnvPrefix("NOTIFY")

	// 设置环境变量映射
	viper.BindEnv("github.token", "GITHUB_TOKEN")
	viper.BindEnv("notifications.dingtalk.webhook_url", "DINGTALK_WEBHOOK")
	viper.BindEnv("notifications.dingtalk.secret", "DINGTALK_SECRET")
	viper.BindEnv("notifications.telegram.bot_token", "TELEGRAM_BOT_TOKEN")
	viper.BindEnv("notifications.telegram.chat_id", "TELEGRAM_CHAT_ID")
	viper.BindEnv("schedule.interval", "SCHEDULE_INTERVAL")
	viper.BindEnv("github.check_days", "CHECK_DAYS")

	// 读取配置文件
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("读取配置文件失败: %v", err)
		}
	}

	// 解析配置到结构体
	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %v", err)
	}

	// 设置默认模板
	if cfg.Template == "" {
		cfg.Template = DefaultTemplate
	}

	// 设置默认检查间隔
	if cfg.Schedule.Enabled && cfg.Schedule.Interval == "" {
		cfg.Schedule.Interval = DefaultInterval
	}

	// 设置默认检查天数
	if cfg.GitHub.CheckDays <= 0 {
		cfg.GitHub.CheckDays = DefaultCheckDays
	}

	return cfg, nil
}
