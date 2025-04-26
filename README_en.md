# Notify

A GitHub repository release notification service that sends repository updates to DingTalk and Telegram.

## Features

- Monitor changes in specified GitHub repositories
- Support for monitoring multiple repositories
- Selectively monitor specific branches and paths
- Support for DingTalk and Telegram notification channels
- Customizable notification templates
- Flexible scheduling configuration
- Smart DingTalk message rate limit management

## Quick Start

### Using Docker

1. Prepare the configuration file
   
   Copy `config/config.example.yaml` to `config.yaml` and modify the configuration as needed.

2. Run with Docker

   ```bash
   docker run -v /path/to/config.yaml:/app/config/config.yaml yourusername/notify:latest
   ```

### Building from Source

1. Clone the repository

   ```bash
   git clone https://github.com/yourusername/notify.git
   cd notify
   ```

2. Build the project

   ```bash
   make release
   ```

3. Prepare the configuration file

   ```bash
   cp config/config.example.yaml config.yaml
   # Edit the configuration file
   ```

4. Run the service

   ```bash
   ./release/notify/notify-linux-amd64
   ```

## Command-line Arguments

The program supports the following command-line arguments:

- `-c, --config <file>`: Specify the configuration file path
- `-d, --show-description`: Include version release descriptions in notifications
- `-n, --days <number>`: Check for releases published within the specified number of days (default is 3 days)

Examples:

```bash
# Use a custom configuration file
./notify --config=/path/to/my-config.yaml

# Check for releases in the last 7 days
./notify --days=7

# Show complete release descriptions
./notify --show-description

# Combine multiple parameters
./notify --days=5 --show-description
```

## Configuration

The configuration file uses YAML format and includes the following main sections:

### GitHub Configuration

```yaml
github:
  token: "your-github-token"  # GitHub personal access token
  repos:
    - owner: "example-owner"  # Repository owner
      name: "example-repo"    # Repository name
      branch: "main"          # Branch name (optional)
      paths:                  # Paths to monitor (optional)
        - "docs/"
        - "src/"
  watch_starred: false        # Whether to monitor starred repositories
  watch_organizations: false  # Whether to monitor organization repositories
  check_days: 3               # Check for releases within this many days (default 3)
```

### Notification Configuration

```yaml
notifications:
  dingtalk:
    webhook_url: "https://oapi.dingtalk.com/robot/send?access_token=your-token"
    secret: "your-secret"
  
  telegram:
    bot_token: "your-bot-token"
    chat_id: "your-chat-id"
```

### Notification Templates and Scheduling

```yaml
template: |
  ## {{ .Repository.FullName }} has updates!
  # Template content...

schedule:
  interval: "5m"  # Check interval, default is 5 minutes
```

## API Rate Limit Handling

To comply with GitHub API rate limits, the tool uses the following strategies:

1. **Batch processing requests**: When monitoring many repositories, API requests are processed in batches to avoid triggering secondary rate limits
2. **Default check interval**: Default is set to check every 6 hours to conserve API request quota
3. **Rate limit monitoring**: Displays the current API rate limit status at each run, and gives warnings when quota is low
4. **Automatic pause**: Automatically pauses requests when API rate limit errors are encountered

> Note: GitHub's authenticated user API rate limit is 5,000 requests per hour. Using GitHub App installation tokens can provide higher limits.
> If you need to monitor a large number of repositories, it's recommended to set the check interval to a longer time or use a GitHub App installation token.
>
> When using the auto-monitoring feature, please ensure you provide sufficient GitHub API permissions. For monitoring organization repositories, the token used needs to have appropriate organization access permissions.

## DingTalk Rate Limit Management

DingTalk bots have specific rate limits:

- Each bot can send a maximum of 20 messages per minute
- If this limit is exceeded, the bot will be throttled for 10 minutes

This tool implements intelligent rate limit management:

- Automatically sends notifications in batches to avoid triggering rate limits
- Adds appropriate delays between batches
- Automatically waits during the cooldown period when rate limiting is encountered
- Provides clear rate limiting prompts and suggestions for resolution

When monitoring a large number of repositories or when there are many updates, it's recommended to:

1. Increase the check interval of scheduled tasks (schedule.interval)
2. Reduce the number of repositories monitored at once
3. Split notifications across multiple instances using different DingTalk bots

## License

MIT 