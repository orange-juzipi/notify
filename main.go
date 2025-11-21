package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/orange-juzipi/notify/config"
	"github.com/orange-juzipi/notify/internal/util"
	"github.com/orange-juzipi/notify/pkg/github"
	"github.com/orange-juzipi/notify/pkg/notifier"
	"github.com/robfig/cron/v3"
	"github.com/spf13/cobra"
)

func main() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

var (
	configFile      string
	showDescription bool
	checkDays       int
)

// RootCmd 表示没有子命令时的基础命令
var RootCmd = &cobra.Command{
	Use:   "notify",
	Short: "GitHub仓库版本发布通知工具",
	Long: `Notify 是一个GitHub仓库版本发布通知工具，支持钉钉和Telegram通知渠道。
可以通过配置文件或环境变量设置要监控的仓库和通知方式。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// 创建文件锁，防止多个实例同时运行
		lock, err := util.NewFileLock("")
		if err != nil {
			return fmt.Errorf("创建文件锁失败: %v", err)
		}

		// 尝试获取锁
		err = lock.Lock()
		if err != nil {
			return fmt.Errorf("⚠️  %v\n提示：请检查是否有其他 notify 进程正在运行", err)
		}
		defer lock.Unlock()

		fmt.Printf("✓ 获取进程锁成功 (PID: %d)\n", os.Getpid())

		// 加载配置
		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			return fmt.Errorf("加载配置失败: %v", err)
		}

		// 如果命令行参数设置了检查天数，覆盖配置文件中的设置
		if cmd.Flags().Changed("days") {
			cfg.GitHub.CheckDays = checkDays
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
	// 添加是否显示描述的标志
	RootCmd.PersistentFlags().BoolVarP(&showDescription, "show-description", "d", false, "是否在通知中显示仓库版本描述信息")
	// 添加检查天数的标志
	RootCmd.PersistentFlags().IntVarP(&checkDays, "days", "n", config.DefaultCheckDays, "检查最近多少天内的版本发布")
}

// runOnce 执行一次检查
func runOnce(cfg *config.Config) error {
	// 检查新版本
	releases, err := github.CheckForNewReleases(cfg, showDescription)
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

	// 打印发现的版本数量
	fmt.Printf("找到 %d 个新版本发布，准备发送通知...\n", len(releases))

	if len(releases) > 20 {
		fmt.Println("警告: 发现的版本超过20个，将会分批发送以避免触发钉钉的速率限制（每分钟最多20条消息）")
	}

	// 发送通知
	errors := manager.NotifyAll(releases)
	if len(errors) > 0 {
		// 分析错误以提供更好的反馈
		var rateLimitErrors, otherErrors []error

		for _, err := range errors {
			if strings.Contains(err.Error(), "速率限制") ||
				strings.Contains(err.Error(), "频率超过限制") {
				rateLimitErrors = append(rateLimitErrors, err)
			} else {
				otherErrors = append(otherErrors, err)
			}
		}

		// 打印速率限制错误
		if len(rateLimitErrors) > 0 {
			fmt.Println("\n⚠️ 触发了钉钉机器人的速率限制（每分钟最多20条消息）")
			fmt.Println("建议：")
			fmt.Println("- 减少单次监控的仓库数量")
			fmt.Println("- 增加定时任务的时间间隔")
			fmt.Println("- 下一次通知将在限流冷却期（10分钟）后恢复")
		}

		// 打印其他错误
		for _, err := range otherErrors {
			fmt.Printf("发送通知失败: %v\n", err)
		}

		if len(otherErrors) > 0 {
			return fmt.Errorf("部分通知发送失败")
		}
		return fmt.Errorf("由于速率限制，部分通知发送失败")
	}

	fmt.Printf("成功发送了 %d 个版本发布通知\n", len(releases))
	return nil
}

// runAsScheduler 作为定时任务运行
func runAsScheduler(cfg *config.Config) error {
	// 设置信号处理
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	if cfg.Schedule.Cron != "" {
		fmt.Printf("以cron表达式模式运行，表达式: %s\n", cfg.Schedule.Cron)
		c := cron.New(cron.WithSeconds())
		_, err := c.AddFunc(cfg.Schedule.Cron, func() {
			err := runOnce(cfg)
			if err != nil {
				fmt.Printf("定时检查失败: %v\n", err)
			}
		})
		if err != nil {
			return fmt.Errorf("解析cron表达式失败: %v", err)
		}
		// 立即进行第一次检查
		if err := runOnce(cfg); err != nil {
			fmt.Printf("初始检查失败: %v\n", err)
		}
		c.Start()
		defer c.Stop()
		<-signals
		fmt.Println("收到终止信号，程序退出")
		return nil
	}

	return fmt.Errorf("未配置cron表达式，请在配置文件中设置schedule.cron")
}
