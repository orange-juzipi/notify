package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/orange-juzipi/notify/config"
	"github.com/orange-juzipi/notify/pkg/github"
	"github.com/orange-juzipi/notify/pkg/notifier"
	"github.com/spf13/cobra"
)

func main() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

var (
	configFile string
)

// RootCmd 表示没有子命令时的基础命令
var RootCmd = &cobra.Command{
	Use:   "notify",
	Short: "GitHub仓库版本发布通知工具",
	Long: `Notify 是一个GitHub仓库版本发布通知工具，支持钉钉和Telegram通知渠道。
可以通过配置文件或环境变量设置要监控的仓库和通知方式。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// 加载配置
		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			return fmt.Errorf("加载配置失败: %v", err)
		}

		// 如果启用了定时运行
		if cfg.Schedule.Enabled {
			return runAsScheduler(cfg)
		}

		// 否则只运行一次
		return runOnce(cfg)
	},
}

func init() {
	// 添加配置文件标志
	RootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "配置文件路径 (默认为 ./config.yaml 或 ~/.notify/config.yaml)")
}

// runOnce 执行一次检查
func runOnce(cfg *config.Config) error {
	// 检查新版本
	releases, err := github.CheckForNewReleases(cfg)
	if err != nil {
		return fmt.Errorf("检查新版本失败: %v", err)
	}

	if len(releases) == 0 {
		fmt.Println("没有找到新版本")
		return nil
	}

	// 创建通知管理器
	manager, err := notifier.NewManager(cfg)
	if err != nil {
		return fmt.Errorf("创建通知管理器失败: %v", err)
	}

	// 发送通知
	errors := manager.NotifyAll(releases)
	if len(errors) > 0 {
		for _, err := range errors {
			fmt.Printf("发送通知失败: %v\n", err)
		}
		return fmt.Errorf("部分通知发送失败")
	}

	fmt.Printf("成功发送了 %d 个版本发布通知\n", len(releases))
	return nil
}

// runAsScheduler 作为定时任务运行
func runAsScheduler(cfg *config.Config) error {
	interval, err := time.ParseDuration(cfg.Schedule.Interval)
	if err != nil {
		return fmt.Errorf("解析定时间隔失败: %v", err)
	}

	fmt.Printf("以定时模式运行，检查间隔: %s\n", interval)

	// 设置信号处理
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	// 创建定时器
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// 立即进行第一次检查
	if err := runOnce(cfg); err != nil {
		fmt.Printf("初始检查失败: %v\n", err)
	}

	// 等待信号或定时器
	for {
		select {
		case <-signals:
			fmt.Println("收到终止信号，程序退出")
			return nil
		case <-ticker.C:
			fmt.Printf("定时检查触发: %s\n", time.Now().Format("2006-01-02 15:04:05"))
			if err := runOnce(cfg); err != nil {
				fmt.Printf("定时检查失败: %v\n", err)
				// 继续下一次检查，不中断
			}
		}
	}
}
