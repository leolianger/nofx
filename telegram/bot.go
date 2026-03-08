package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"nofx/api"
	"nofx/config"
	"nofx/logger"
	"nofx/mcp"
	"nofx/store"
	"nofx/telegram/agent"
	"os"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Start initializes and runs the Telegram bot in a blocking supervisor loop.
// Supports hot-reload: when a signal is sent on reloadCh, the bot restarts
// with the latest token (re-read from DB or env). Must be called as a goroutine from main.go.
// Deployment note: uses long-polling (not webhook) — safe for private networks,
// no inbound ports required.
func Start(cfg *config.Config, st *store.Store, reloadCh <-chan struct{}) {
	for {
		token := resolveToken(cfg, st)
		if token == "" {
			logger.Info("Telegram bot disabled (no token configured), waiting for reload signal...")
			// Block until a reload signal arrives, then re-check for a token.
			<-reloadCh
			continue
		}

		stopped := runBot(token, cfg, st)
		if !stopped {
			// Bot exited with an unrecoverable error; do not restart automatically.
			return
		}

		// Bot was stopped cleanly. Wait for a reload signal before restarting.
		select {
		case <-reloadCh:
			logger.Info("Reloading Telegram bot with new token...")
		}
	}
}

// resolveToken returns the bot token, preferring the DB-stored value over the env/config value.
func resolveToken(cfg *config.Config, st *store.Store) string {
	dbCfg, err := st.TelegramConfig().Get()
	if err == nil && dbCfg.BotToken != "" {
		return dbCfg.BotToken
	}
	return cfg.TelegramBotToken
}

// runBot runs the bot until StopReceivingUpdates is called (clean stop → true)
// or a fatal error occurs (false).
func runBot(token string, cfg *config.Config, st *store.Store) bool {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		logger.Errorf("Telegram bot failed to start: %v", err)
		return false
	}
	logger.Infof("Telegram bot @%s started (long-polling mode)", bot.Self.UserName)

	// Determine allowed chat ID:
	// Priority 1: env var TELEGRAM_ADMIN_CHAT_ID (explicit)
	// Priority 2: DB-stored bound chat ID (set by /start)
	// Priority 3: 0 = no binding yet, first /start will bind
	allowedChatID := cfg.TelegramAdminChatID
	if allowedChatID == 0 {
		if id, err := st.TelegramConfig().GetBoundChatID(); err == nil && id != 0 {
			allowedChatID = id
		}
	}

	// Resolve the real user ID: use the first registered user so that bot-made
	// changes (model/exchange configs) are visible in the frontend under that user.
	// Falls back to "default" if no users exist yet (fresh install).
	botUserID := "default"
	if ids, err := st.User().GetAllIDs(); err == nil && len(ids) > 0 {
		botUserID = ids[0]
	}

	// Generate a bot JWT for authenticated API calls. Re-generated on each bot start.
	botToken, err := agent.GenerateBotToken(botUserID)
	if err != nil {
		logger.Errorf("Failed to generate bot JWT: %v", err)
		return false
	}

	// Wire the AI agent manager. API docs are auto-generated from registered routes.
	agents := agent.NewManager(cfg.APIServerPort, botToken, botUserID,
		func() mcp.AIClient { return newLLMClient(st, botUserID) },
		api.GetAPIDocs(),
	)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}
		chatID := update.Message.Chat.ID
		text := update.Message.Text

		// Handle /start: auto-bind or welcome with setup guide
		if text == "/start" {
			if allowedChatID == 0 {
				username := update.Message.From.UserName
				if err := st.TelegramConfig().BindUser(chatID, "@"+username); err != nil {
					logger.Errorf("Failed to bind Telegram user: %v", err)
					sendMsg(bot, chatID, "绑定失败，请查看服务器日志。")
					continue
				}
				allowedChatID = chatID
				logger.Infof("Telegram bound to @%s (chatID: %d)", username, chatID)
			} else if chatID != allowedChatID {
				sendMsg(bot, chatID, "该机器人已被其他用户绑定。")
				continue
			} else {
				agents.Reset(chatID)
			}
			sendMsg(bot, chatID, buildSetupGuide(st, botUserID, cfg.APIServerPort, botToken))
			continue
		}

		// Handle /help
		if text == "/help" {
			sendMsg(bot, chatID, helpMessage())
			continue
		}

		// Access control
		if allowedChatID != 0 && chatID != allowedChatID {
			sendMsg(bot, chatID, "无权限访问。")
			continue
		}
		if allowedChatID == 0 {
			sendMsg(bot, chatID, "请先发送 /start 绑定账号。")
			continue
		}
		if text == "" {
			continue
		}

		// Direct setup commands — handled without LLM so they work even before
		// an AI model is configured. Format: "配置 <provider> <api-key>"
		if reply, handled := tryHandleSetupCommand(text, cfg.APIServerPort, botToken, st, botUserID); handled {
			sendMsg(bot, chatID, reply)
			continue
		}

		// Check if AI model is configured before entering agent loop.
		if newLLMClient(st, botUserID) == nil {
			sendMsg(bot, chatID, buildSetupGuide(st, botUserID, cfg.APIServerPort, botToken))
			continue
		}

		// Send a placeholder immediately, then stream-edit as reply arrives.
		go func(chatID int64, text string) {
			// Send ⏳ placeholder so the user sees an instant response.
			sent, err := bot.Send(tgbotapi.NewMessage(chatID, "⏳"))
			placeholderID := 0
			if err == nil {
				placeholderID = sent.MessageID
			}

			// Rate-limited edit helper: edits the placeholder at most once per second.
			// Exception: "⏳" thinking-indicator resets always go through immediately
			// so the user always sees the state change between agent iterations.
			var (
				mu       sync.Mutex
				lastEdit time.Time
			)
			onChunk := func(accumulated string) {
				if placeholderID == 0 {
					return
				}
				mu.Lock()
				defer mu.Unlock()
				isThinking := accumulated == "⏳"
				if !isThinking && time.Since(lastEdit) < time.Second {
					return
				}
				lastEdit = time.Now()
				edit := tgbotapi.NewEditMessageText(chatID, placeholderID, accumulated)
				bot.Send(edit) //nolint:errcheck
			}

			reply := agents.Run(chatID, text, onChunk)

			// Final edit: use Markdown, fall back to plain text on parse error.
			if placeholderID != 0 {
				edit := tgbotapi.NewEditMessageText(chatID, placeholderID, reply)
				edit.ParseMode = "Markdown"
				if _, err := bot.Send(edit); err != nil {
					edit2 := tgbotapi.NewEditMessageText(chatID, placeholderID, reply)
					bot.Send(edit2) //nolint:errcheck
				}
			} else {
				msg := tgbotapi.NewMessage(chatID, reply)
				msg.ParseMode = "Markdown"
				if _, err := bot.Send(msg); err != nil {
					msg.ParseMode = ""
					bot.Send(msg) //nolint:errcheck
				}
			}
		}(chatID, text)
	}

	// updates channel was closed — bot stopped cleanly
	return true
}

func sendMsg(bot *tgbotapi.BotAPI, chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	bot.Send(msg) //nolint:errcheck
}

// newLLMClient builds an LLM client for the agent using the bound user's enabled model.
// Priority: bound user's DB model > environment variables.
// Uses provider-specific constructors to ensure correct default base URLs and models.
func newLLMClient(st *store.Store, userID string) mcp.AIClient {
	// 1. Try the bound user's enabled model from DB (configured via Web UI)
	if model, err := st.AIModel().GetDefault(userID); err == nil {
		apiKey := string(model.APIKey)
		if apiKey != "" {
			client := clientForProvider(model.Provider)
			client.SetAPIKey(apiKey, model.CustomAPIURL, model.CustomModelName)
			logger.Infof("Telegram agent: provider=%s user_id=%s model=%q url=%q",
				model.Provider, model.UserID, model.CustomModelName, model.CustomAPIURL)
			return client
		}
		logger.Warnf("Telegram: DB model found (provider=%s) but API key is empty after decryption", model.Provider)
	} else {
		logger.Warnf("Telegram: no enabled model for user %s (%v), trying env vars", userID, err)
	}

	// 2. Fall back to environment variables
	for _, pair := range []struct{ provider, key, url string }{
		{"deepseek", os.Getenv("DEEPSEEK_API_KEY"), mcp.DefaultDeepSeekBaseURL},
		{"openai", os.Getenv("OPENAI_API_KEY"), ""},
		{"claude", os.Getenv("ANTHROPIC_API_KEY"), ""},
	} {
		if pair.key != "" {
			client := clientForProvider(pair.provider)
			client.SetAPIKey(pair.key, pair.url, "")
			logger.Infof("Telegram agent: using %s from env var", pair.provider)
			return client
		}
	}

	logger.Warn("Telegram: no AI key found in DB or env — agent will fail. Configure a model in the Web UI.")
	return mcp.NewDeepSeekClient() // return a typed client so caller gets a clear API error
}

// clientForProvider returns the appropriate provider-specific client.
// Each constructor sets correct default base URL and model for that provider.
func clientForProvider(provider string) mcp.AIClient {
	switch provider {
	case "openai":
		return mcp.NewOpenAIClient()
	case "deepseek":
		return mcp.NewDeepSeekClient()
	case "claude":
		return mcp.NewClaudeClient()
	case "qwen":
		return mcp.NewQwenClient()
	case "kimi":
		return mcp.NewKimiClient()
	case "grok":
		return mcp.NewGrokClient()
	case "gemini":
		return mcp.NewGeminiClient()
	default:
		// Unknown/custom provider: fall back to OpenAI-compatible format.
		return mcp.NewDeepSeekClient()
	}
}

// buildSetupGuide checks the current configuration state and returns a contextual
// onboarding message. Called on every /start so the user always knows what to do next.
func buildSetupGuide(st *store.Store, userID string, apiPort int, botToken string) string {
	// Step 1: AI model configured?
	if _, err := st.AIModel().GetDefault(userID); err != nil {
		return `🤖 *NOFX 个人 AI 交易助手*

欢迎！在开始交易前，需要先配置 AI 模型。

*第一步：配置 AI 模型*

选择你有账号的服务商，发送以下格式的消息：

` + "```" + `
配置 deepseek  你的API-Key
配置 openai    你的API-Key
配置 claude    你的API-Key
配置 qwen      你的API-Key
配置 kimi      你的API-Key
配置 grok      你的API-Key
配置 gemini    你的API-Key
` + "```" + `

*推荐*：DeepSeek 价格最低，效果很好
获取 Key：https://platform.deepseek.com/api_keys`
	}

	// Step 2: Exchange configured?
	exchanges, _ := st.Exchange().List(userID)
	hasEnabled := false
	for _, e := range exchanges {
		if e.Enabled {
			hasEnabled = true
			break
		}
	}
	if !hasEnabled {
		return `✅ AI 模型已配置！

*第二步：配置交易所*

直接发消息告诉我交易所信息，例如：

_"帮我配置 OKX 交易所，API Key 是 xxx，Secret 是 xxx，Passphrase 是 xxx"_

_"帮我配置 Binance，API Key 是 xxx，Secret Key 是 xxx"_

_"帮我配置 Bybit，API Key 是 xxx，Secret Key 是 xxx"_

交易所 API Key 去交易所官网 → 账户设置 → API 管理 → 新建 Key（需要开启合约交易权限）`
	}

	// All configured — show full capabilities
	return `✅ *NOFX 交易助手已就绪*

直接用自然语言告诉我你要做什么：

*查询*
_"查看我的持仓"_、_"查看账户余额"_

*创建并启动交易*
_"帮我创建一个 BTC 趋势策略并跑起来"_
_"创建保守型策略，只交易 BTC 和 ETH"_

*控制*
_"启动交易员"_、_"暂停交易员"_

*查看数据*
_"查看今天的交易记录"_、_"查看盈亏统计"_

发送 /start 重置对话 | /help 查看更多`
}

// tryHandleSetupCommand handles "配置 <provider> <api-key>" commands directly
// without going through the LLM. This allows AI model setup even before any
// model is configured (bootstrapping problem).
func tryHandleSetupCommand(text string, apiPort int, botToken string, st *store.Store, userID string) (string, bool) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "配置 ") && !strings.HasPrefix(strings.ToLower(text), "setup ") {
		return "", false
	}

	parts := strings.Fields(text)
	if len(parts) < 3 {
		return "格式：配置 <服务商> <API-Key>\n例如：配置 deepseek sk-xxxxxxxxx", true
	}

	provider := strings.ToLower(parts[1])
	apiKey := parts[2]

	validProviders := map[string]bool{
		"openai": true, "deepseek": true, "claude": true,
		"qwen": true, "kimi": true, "grok": true, "gemini": true,
	}
	if !validProviders[provider] {
		return fmt.Sprintf("不支持的服务商：%s\n支持：openai / deepseek / claude / qwen / kimi / grok / gemini", provider), true
	}

	// Call PUT /api/models directly without LLM.
	body, _ := json.Marshal(map[string]any{
		"models": map[string]any{
			provider: map[string]any{
				"enabled": true,
				"api_key": apiKey,
			},
		},
	})
	req, err := http.NewRequest("PUT", fmt.Sprintf("http://127.0.0.1:%d/api/models", apiPort), bytes.NewReader(body))
	if err != nil {
		return "配置请求失败，请稍后重试", true
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+botToken)

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return "无法连接到服务，请确认服务正常运行", true
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Sprintf("配置失败（%d）：%s", resp.StatusCode, string(respBody)), true
	}

	logger.Infof("Bot: setup command configured provider=%s", provider)
	return fmt.Sprintf("✅ %s 配置成功！\n\n发送 /start 查看下一步", provider), true
}

func helpMessage() string {
	return `*NOFX 使用指南*

*查询*
- "查看我的持仓"
- "查看账户余额"
- "列出我的交易员"

*交易控制*
- "启动交易员"
- "暂停 xxx 交易员"

*创建策略*
- "帮我创建 BTC 趋势策略并跑起来"
- "创建保守型策略，BTC ETH，止损 8%"

*配置*（不需要 AI，直接发）
- "配置 deepseek sk-xxxx"
- "帮我配置 OKX 交易所，Key 是 xxx"

*命令*
/start - 重置对话 / 查看配置状态
/help  - 显示此帮助

支持中文和英文，直接说你想做什么就行。`
}
