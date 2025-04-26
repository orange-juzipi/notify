package github

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/go-github/v71/github"
	"github.com/orange-juzipi/notify/config"
	"github.com/orange-juzipi/notify/internal/util"
	"golang.org/x/oauth2"
)

// ReleaseInfo 包含版本发布信息
type ReleaseInfo struct {
	Owner       string
	Repository  string
	TagName     string
	Name        string
	Description string
	HTMLURL     string
	PublishedAt time.Time
}

// Client GitHub客户端
type Client struct {
	client *github.Client
	ctx    context.Context
	store  *util.StateStore
}

// NewClient 创建新的GitHub客户端
func NewClient(token string, storePath string) (*Client, error) {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)

	store, err := util.NewStateStore(storePath)
	if err != nil {
		return nil, fmt.Errorf("创建状态存储失败: %v", err)
	}

	return &Client{
		client: github.NewClient(tc),
		ctx:    ctx,
		store:  store,
	}, nil
}

// GetLatestRelease 获取仓库最新的Release
func (c *Client) GetLatestRelease(owner, repo string, showDescription bool) (*ReleaseInfo, error) {
	release, resp, err := c.client.Repositories.GetLatestRelease(c.ctx, owner, repo)
	if err != nil {
		// 检查是否是404错误（没有release）
		if resp != nil && resp.StatusCode == 404 {
			// 对于没有release的仓库，返回nil而不是错误
			return nil, nil
		}
		return nil, fmt.Errorf("获取最新版本失败: %v", err)
	}

	tagName := release.GetTagName()
	publishedTime := release.GetPublishedAt().Time

	// 检查是否在7天内发布（基于中国时间）
	// 获取中国时区（UTC+8）
	chinaLoc := time.FixedZone("CST", 8*60*60)
	sevenDaysAgo := time.Now().In(chinaLoc).AddDate(0, 0, -7)

	if publishedTime.Before(sevenDaysAgo) {
		// 如果发布时间早于7天前，则忽略这个版本
		return nil, nil
	}

	// 检查是否为新版本
	if !c.store.IsNewRelease(owner, repo, tagName) {
		return nil, nil // 返回nil表示没有新版本
	}

	// 更新状态
	if err := c.store.UpdateState(owner, repo, tagName); err != nil {
		fmt.Printf("警告: 更新版本状态失败: %v\n", err)
		// 继续处理，不中断流程
	}

	releaseInfo := &ReleaseInfo{
		Owner:       owner,
		Repository:  repo,
		TagName:     tagName,
		Name:        release.GetName(),
		HTMLURL:     release.GetHTMLURL(),
		PublishedAt: release.GetPublishedAt().Time,
	}

	// 根据showDescription参数决定是否包含描述信息
	if showDescription {
		releaseInfo.Description = release.GetBody()
	}

	return releaseInfo, nil
}

// CheckForNewReleases 检查所有配置的仓库是否有新版本
func CheckForNewReleases(cfg *config.Config, showDescription bool) ([]*ReleaseInfo, error) {
	client, err := NewClient(cfg.GitHub.Token, "")
	if err != nil {
		return nil, fmt.Errorf("创建GitHub客户端失败: %v", err)
	}

	// 尝试获取速率限制信息
	rl, resp, err := client.client.RateLimit.Get(client.ctx)
	if err == nil && resp != nil && rl != nil && rl.Core != nil {
		remaining := rl.Core.Remaining
		resetTime := rl.Core.Reset.Time

		fmt.Printf("GitHub API 速率限制状态: %d/%d，重置时间：%s\n",
			remaining, rl.Core.Limit,
			resetTime.Format(time.DateTime))

		// 如果剩余请求数很少，提醒用户
		if remaining < 50 {
			fmt.Printf("⚠️ 警告: GitHub API 请求配额不足，仅剩 %d 次请求\n", remaining)
		}
	}

	// 显示仅检查最近7天的提示
	chinaLoc := time.FixedZone("CST", 8*60*60)
	sevenDaysAgo := time.Now().In(chinaLoc).AddDate(0, 0, -7)
	fmt.Printf("仅检查最近7天（%s 之后）发布的版本\n", sevenDaysAgo.Format("2006-01-02"))

	// 使用map去重，避免重复监控同一个仓库
	repoMap := make(map[string]config.RepoConfig)

	// 如果配置了手动指定的仓库，添加到待检查列表
	if len(cfg.GitHub.Repos) > 0 {
		for _, repo := range cfg.GitHub.Repos {
			key := fmt.Sprintf("%s/%s", repo.Owner, repo.Name)
			repoMap[key] = repo
		}
	}

	// 如果启用了自动监控用户仓库
	if cfg.GitHub.AutoWatchUser {
		fmt.Println("正在获取用户仓库列表...")
		userRepos, err := client.getUserRepositories(cfg.GitHub.OnlyWithReleases)
		if err != nil {
			fmt.Printf("获取用户仓库列表失败: %v\n", err)
		} else {
			fmt.Printf("找到 %d 个用户仓库\n", len(userRepos))
			for _, repo := range userRepos {
				key := fmt.Sprintf("%s/%s", repo.Owner, repo.Name)
				repoMap[key] = repo
			}
		}
	}

	// 如果启用了监控star的仓库
	if cfg.GitHub.WatchStarred {
		fmt.Println("正在获取用户已star的仓库列表...")
		starredRepos, err := client.getUserStarredRepositories(cfg.GitHub.OnlyWithReleases)
		if err != nil {
			fmt.Printf("获取用户已star的仓库列表失败: %v\n", err)
		} else {
			fmt.Printf("找到 %d 个已star的仓库\n", len(starredRepos))
			for _, repo := range starredRepos {
				key := fmt.Sprintf("%s/%s", repo.Owner, repo.Name)
				repoMap[key] = repo
			}
		}
	}

	// 如果配置了需要监控的组织
	if len(cfg.GitHub.WatchOrgs) > 0 {
		for _, org := range cfg.GitHub.WatchOrgs {
			fmt.Printf("正在获取组织 %s 的仓库列表...\n", org)
			orgRepos, err := client.getOrgRepositories(org, cfg.GitHub.OnlyWithReleases)
			if err != nil {
				fmt.Printf("获取组织 %s 的仓库列表失败: %v\n", org, err)
				continue
			}
			fmt.Printf("找到 %d 个组织仓库\n", len(orgRepos))
			for _, repo := range orgRepos {
				key := fmt.Sprintf("%s/%s", repo.Owner, repo.Name)
				repoMap[key] = repo
			}
		}
	}

	// 将去重后的仓库列表转换为切片
	var repoConfigs []config.RepoConfig
	for _, repo := range repoMap {
		repoConfigs = append(repoConfigs, repo)
	}

	// 如果没有找到要监控的仓库
	if len(repoConfigs) == 0 {
		// 检查是否是因为只过滤了有release的仓库导致的
		if cfg.GitHub.OnlyWithReleases && (cfg.GitHub.AutoWatchUser || cfg.GitHub.WatchStarred) {
			fmt.Println("\n注意: 没有找到任何有release的仓库。")
			fmt.Println("如果您确定要监控没有release的仓库，请在配置中设置 only_with_releases: false")

			// 尝试获取所有仓库（包括没有release的）
			var allRepos []config.RepoConfig

			if cfg.GitHub.AutoWatchUser {
				userRepos, err := client.getUserRepositories(false)
				if err == nil && len(userRepos) > 0 {
					for _, repo := range userRepos {
						key := fmt.Sprintf("%s/%s", repo.Owner, repo.Name)
						if _, exists := repoMap[key]; !exists {
							repoMap[key] = repo
							allRepos = append(allRepos, repo)
						}
					}
				}
			}

			if cfg.GitHub.WatchStarred && len(allRepos) == 0 {
				starredRepos, err := client.getUserStarredRepositories(false)
				if err == nil && len(starredRepos) > 0 {
					for _, repo := range starredRepos {
						key := fmt.Sprintf("%s/%s", repo.Owner, repo.Name)
						if _, exists := repoMap[key]; !exists {
							repoMap[key] = repo
							allRepos = append(allRepos, repo)
						}
					}
				}
			}

			if len(allRepos) > 0 {
				fmt.Println("\n为了让系统正常运行，将监控所有仓库（不仅限于有release的仓库）:")
				for i, repo := range allRepos {
					if i < 5 { // 只显示前5个
						fmt.Printf("- %s/%s\n", repo.Owner, repo.Name)
					}
				}
				if len(allRepos) > 5 {
					fmt.Printf("  ...以及其他 %d 个仓库\n", len(allRepos)-5)
				}
				repoConfigs = allRepos
			} else {
				return nil, fmt.Errorf("未找到任何仓库，请检查GitHub Token权限或在配置文件中手动指定仓库")
			}
		} else {
			return nil, fmt.Errorf("未配置要监控的仓库，请在配置文件中添加仓库或启用自动监控")
		}
	}

	fmt.Printf("共监控 %d 个仓库，正在并发检查是否有新版本发布...\n", len(repoConfigs))

	var (
		results        []*ReleaseInfo
		noReleaseCount int
		errorCount     int
		rateLimitHit   bool
		mu             sync.Mutex
		wg             sync.WaitGroup
	)

	// 根据GitHub API速率限制，设置合理的并发数
	// 对于认证用户，主要速率限制是每小时5000次请求
	// 考虑到每个仓库检查只需要一个请求，可以适当提高并发度
	// 但设置上限以避免触发二级速率限制（最多每分钟发送900个点的请求）
	concurrencyLimit := 10 // 10个并发请求，避免触发二级速率限制

	// 对于大量仓库，分批处理以避免过快消耗速率限制
	batchSize := 100 // 每批处理100个仓库

	// 处理可能的速率限制重试
	checkRepo := func(r config.RepoConfig) {
		defer wg.Done()

		release, err := client.GetLatestRelease(r.Owner, r.Name, showDescription)

		mu.Lock()
		defer mu.Unlock()

		if err != nil {
			// 检查是否是速率限制错误
			if strings.Contains(err.Error(), "rate limit exceeded") {
				rateLimitHit = true
				fmt.Printf("警告: GitHub API 速率限制已达到，请稍后再试\n")
				return
			}

			fmt.Printf("获取仓库 %s/%s 最新版本失败: %v\n", r.Owner, r.Name, err)
			errorCount++
			return
		}

		// 如果有新版本
		if release != nil {
			fmt.Printf("发现新版本: %s/%s (%s)\n", r.Owner, r.Name, release.TagName)
			results = append(results, release)
		} else {
			noReleaseCount++
		}
	}

	// 分批处理仓库，避免一次性发送过多请求
	for i := 0; i < len(repoConfigs); i += batchSize {
		// 创建信号量控制并发数
		semaphore := make(chan struct{}, concurrencyLimit)

		end := i + batchSize
		if end > len(repoConfigs) {
			end = len(repoConfigs)
		}

		fmt.Printf("正在处理第 %d 到 %d 个仓库...\n", i+1, end)

		// 处理当前批次的仓库
		for j := i; j < end; j++ {
			if rateLimitHit {
				fmt.Println("已达到API速率限制，暂停检查")
				break
			}

			wg.Add(1)
			// 放入信号量
			semaphore <- struct{}{}

			go func(r config.RepoConfig) {
				defer func() { <-semaphore }() // 释放信号量
				checkRepo(r)
			}(repoConfigs[j])
		}

		// 等待当前批次完成
		wg.Wait()

		if rateLimitHit {
			fmt.Println("由于API速率限制，部分仓库未能检查。请稍后再试。")
			break
		}

		// 在批次之间添加短暂延迟，避免触发二级速率限制
		if end < len(repoConfigs) {
			fmt.Println("等待1秒继续下一批检查...")
			time.Sleep(1 * time.Second)
		}
	}

	fmt.Printf("\n检查完成: 共 %d 个仓库\n", len(repoConfigs))
	if rateLimitHit {
		fmt.Printf("- 由于达到GitHub API速率限制，部分仓库未能检查\n")
	}
	fmt.Printf("- 发现 %d 个最近7天内发布的新版本\n", len(results))
	fmt.Printf("- %d 个仓库没有release或发布时间超过7天\n", noReleaseCount)
	if errorCount > 0 {
		fmt.Printf("- %d 个仓库检查失败\n", errorCount)
	}

	if len(results) == 0 {
		fmt.Println("\n提示: 未发现任何7天内发布的新版本。如果您想测试通知功能，可以:")
		fmt.Println("1. 在您的任意GitHub仓库中创建一个新的release")
		fmt.Println("2. 修改状态文件 ~/.notify/state.json 删除对应仓库的记录")
		fmt.Println("3. 手动在配置文件中添加要监控的特定仓库")
	}

	return results, nil
}

// getUserRepositories 获取授权用户的所有仓库
func (c *Client) getUserRepositories(onlyWithReleases bool) ([]config.RepoConfig, error) {
	opt := &github.RepositoryListByAuthenticatedUserOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var allRepos []config.RepoConfig

	for {
		repos, resp, err := c.client.Repositories.ListByAuthenticatedUser(c.ctx, opt)
		if err != nil {
			return nil, fmt.Errorf("获取用户仓库列表失败: %v", err)
		}

		for _, repo := range repos {
			// 跳过fork的仓库
			if repo.GetFork() {
				continue
			}

			allRepos = append(allRepos, config.RepoConfig{
				Owner: repo.GetOwner().GetLogin(),
				Name:  repo.GetName(),
			})
		}

		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	// 如果不需要过滤，直接返回
	if !onlyWithReleases {
		return allRepos, nil
	}

	// 使用协程并发检查是否有release
	return c.filterReposWithReleases(allRepos, "用户")
}

// getUserStarredRepositories 获取用户已star的仓库
func (c *Client) getUserStarredRepositories(onlyWithReleases bool) ([]config.RepoConfig, error) {
	opt := &github.ActivityListStarredOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var allRepos []config.RepoConfig

	for {
		repos, resp, err := c.client.Activity.ListStarred(c.ctx, "", opt)
		if err != nil {
			return nil, fmt.Errorf("获取用户已star的仓库列表失败: %v", err)
		}

		for _, repo := range repos {
			// 确保获取的是仓库对象，而不是其他类型
			repository := repo.GetRepository()
			if repository != nil {
				allRepos = append(allRepos, config.RepoConfig{
					Owner: repository.GetOwner().GetLogin(),
					Name:  repository.GetName(),
				})
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	// 如果不需要过滤，直接返回
	if !onlyWithReleases {
		return allRepos, nil
	}

	// 使用协程并发检查是否有release
	return c.filterReposWithReleases(allRepos, "已star")
}

// getOrgRepositories 获取指定组织的所有仓库
func (c *Client) getOrgRepositories(org string, onlyWithReleases bool) ([]config.RepoConfig, error) {
	opt := &github.RepositoryListByOrgOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var allRepos []config.RepoConfig

	for {
		repos, resp, err := c.client.Repositories.ListByOrg(c.ctx, org, opt)
		if err != nil {
			return nil, fmt.Errorf("获取组织仓库列表失败: %v", err)
		}

		for _, repo := range repos {
			allRepos = append(allRepos, config.RepoConfig{
				Owner: org,
				Name:  repo.GetName(),
			})
		}

		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	// 如果不需要过滤，直接返回
	if !onlyWithReleases {
		return allRepos, nil
	}

	// 使用协程并发检查是否有release
	return c.filterReposWithReleases(allRepos, "组织")
}

// filterReposWithReleases 使用并发方式过滤有release的仓库
func (c *Client) filterReposWithReleases(allRepos []config.RepoConfig, repoType string) ([]config.RepoConfig, error) {
	if len(allRepos) == 0 {
		return nil, nil
	}

	fmt.Printf("正在检查 %d 个%s仓库是否有release...\n", len(allRepos), repoType)

	var (
		filteredRepos []config.RepoConfig
		mu            sync.Mutex
		wg            sync.WaitGroup
		checked       int32
		batchSize     = 20 // 每批处理的仓库数量
		workerCount   = 20 // 最大并发worker数
		semaphore     = make(chan struct{}, workerCount)
		progressCh    = make(chan int, len(allRepos))
		done          = make(chan struct{})
	)

	// 启动进度报告协程
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-progressCh:
				atomic.AddInt32(&checked, 1)
			case <-ticker.C:
				current := atomic.LoadInt32(&checked)
				if current > 0 {
					fmt.Printf("已检查 %d/%d 个%s仓库...\n", current, len(allRepos), repoType)
				}
			case <-done:
				return
			}
		}
	}()

	// 批量处理仓库
	for i := 0; i < len(allRepos); i += batchSize {
		end := i + batchSize
		if end > len(allRepos) {
			end = len(allRepos)
		}

		batch := allRepos[i:end]
		for _, repo := range batch {
			wg.Add(1)
			semaphore <- struct{}{} // 获取信号量

			go func(r config.RepoConfig) {
				defer func() {
					<-semaphore // 释放信号量
					wg.Done()
				}()

				_, resp, _ := c.client.Repositories.GetLatestRelease(c.ctx, r.Owner, r.Name)

				// 如果有release (HTTP 200) 或者API错误但不是404
				if resp != nil && resp.StatusCode != 404 {
					mu.Lock()
					filteredRepos = append(filteredRepos, r)
					mu.Unlock()
				}

				progressCh <- 1
			}(repo)
		}
	}

	wg.Wait()
	close(done) // 通知进度报告协程结束

	fmt.Printf("%s仓库过滤完成: 共 %d 个，有release的 %d 个\n", repoType, len(allRepos), len(filteredRepos))
	return filteredRepos, nil
}
