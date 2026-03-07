package agent

import (
	"encoding/json"
	"fmt"
	"nofx/auth"
	"nofx/logger"
	"nofx/mcp"
	"nofx/telegram/session"
	"strings"
)

const maxIterations = 10

// Agent is a stateful AI agent for one Telegram chat.
// It has a single tool (api_call) and an unbounded decision loop.
type Agent struct {
	apiTool      *apiCallTool
	getLLM       func() mcp.AIClient
	memory       *session.Memory
	systemPrompt string
	userID       string
}

// New creates an Agent for one chat session.
func New(apiPort int, botToken, userID string, getLLM func() mcp.AIClient, systemPrompt string) *Agent {
	return &Agent{
		apiTool:      newAPICallTool(apiPort, botToken),
		getLLM:       getLLM,
		memory:       session.NewMemory(getLLM()),
		systemPrompt: systemPrompt,
		userID:       userID,
	}
}

// GenerateBotToken creates a long-lived JWT for the bot's internal API calls.
// userID must match the actual registered user's ID so that bot-made changes
// are visible in the frontend (they share the same user namespace).
func GenerateBotToken(userID string) (string, error) {
	return auth.GenerateJWT(userID, "bot@internal")
}

// buildAccountContext fetches the live account state (models, exchanges, strategies, traders,
// and per-trader account summary + statistics) via the local API and returns it as a formatted
// string for injection into the LLM context. This gives the LLM immediate awareness of what
// is already configured and the current financial state, so it never asks the user for
// information that already exists.
func (a *Agent) buildAccountContext() string {
	type q struct {
		label string
		path  string
	}
	queries := []q{
		{"AI Models", "/api/models"},
		{"Exchanges", "/api/exchanges"},
		{"Strategies", "/api/strategies"},
		{"Traders", "/api/my-traders"},
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[Current Account State - Authenticated User ID: %s]\n\n", a.userID))

	var tradersJSON string
	for _, query := range queries {
		result := a.apiTool.execute(&apiRequest{Method: "GET", Path: query.path})
		sb.WriteString(fmt.Sprintf("%s:\n%s\n\n", query.label, result))
		if query.path == "/api/my-traders" {
			tradersJSON = result
		}
	}

	// For each running trader, fetch real-time account balance and trading statistics.
	var traders []struct {
		TraderID  string `json:"trader_id"`
		Name      string `json:"trader_name"`
		IsRunning bool   `json:"is_running"`
	}
	if err := json.Unmarshal([]byte(tradersJSON), &traders); err == nil {
		for _, t := range traders {
			if !t.IsRunning {
				continue
			}
			acct := a.apiTool.execute(&apiRequest{Method: "GET", Path: "/api/account?trader_id=" + t.TraderID})
			sb.WriteString(fmt.Sprintf("Account [%s] (trader_id=%s):\n%s\n\n", t.Name, t.TraderID, acct))

			stats := a.apiTool.execute(&apiRequest{Method: "GET", Path: "/api/statistics?trader_id=" + t.TraderID})
			sb.WriteString(fmt.Sprintf("Statistics [%s] (trader_id=%s):\n%s\n\n", t.Name, t.TraderID, stats))
		}
	}

	return sb.String()
}

// Run processes one user message through the agent loop.
// Loop: LLM decides -> if <api_call>: execute, append result, loop -> if no tag: return reply.
//
// On the first message of a conversation, the current account state (models, exchanges,
// strategies, traders) is automatically fetched and injected so the LLM knows what is
// already configured without asking the user to repeat themselves.
//
// onChunk is optional. When non-nil, each LLM call is streamed:
//   - Chunks are forwarded to onChunk until an <api_call> tag appears in the accumulated text.
//   - After an api_call iteration completes, onChunk("⏳") resets the display to a thinking indicator.
//   - The final reply is streamed progressively via onChunk.
func (a *Agent) Run(userMessage string, onChunk func(string)) string {
	llm := a.getLLM()
	if llm == nil {
		return "AI assistant unavailable. Please configure an AI model in the Web UI."
	}

	// Build turn messages: history context prefix + current user message.
	// On the very first message (no history), prepend a live account state snapshot so the
	// LLM immediately knows what models, exchanges, strategies, and traders are configured.
	histCtx := a.memory.BuildContext()
	var firstMsg string
	if histCtx == "" {
		// First message in this conversation — fetch and inject account state.
		accountCtx := a.buildAccountContext()
		firstMsg = accountCtx + "\n[User Message]\n" + userMessage
	} else {
		firstMsg = histCtx + "\n---\nUser: " + userMessage
	}
	turnMsgs := []mcp.Message{mcp.NewUserMessage(firstMsg)}

	var lastResp string

	for i := 0; i < maxIterations; i++ {
		req, err := mcp.NewRequestBuilder().
			WithSystemPrompt(a.systemPrompt).
			AddConversationHistory(turnMsgs).
			Build()
		if err != nil {
			logger.Errorf("Agent: failed to build request: %v", err)
			break
		}

		var resp string
		if onChunk != nil {
			// Stream this call; suppress chunks once an <api_call> tag appears.
			// Also hold back the last (len("<api_call>")-1) chars of accumulated text to
			// avoid showing partial opening tags (e.g. "<", "<ap") before we can detect them.
			const tagLen = len("<api_call>") // 10
			const safeOffset = tagLen - 1    // 9: max prefix of tag we might have received

			var apiTagSeen bool
			resp, err = llm.CallWithRequestStream(req, func(accumulated string) {
				if apiTagSeen {
					return
				}
				if idx := strings.Index(accumulated, "<api_call>"); idx >= 0 {
					apiTagSeen = true
					// Forward only the text that appeared before the tag.
					if display := strings.TrimSpace(accumulated[:idx]); display != "" {
						onChunk(display)
					}
					return
				}
				// Forward only the "safe" prefix — hold back the last safeOffset chars
				// in case they are the beginning of an <api_call> tag.
				if safe := len(accumulated) - safeOffset; safe > 0 {
					onChunk(accumulated[:safe])
				}
			})
		} else {
			resp, err = llm.CallWithRequest(req)
		}
		if err != nil {
			logger.Errorf("Agent: LLM call failed (iteration %d): %v", i+1, err)
			return "AI assistant temporarily unavailable. Please try again."
		}
		lastResp = resp

		apiReq, textBefore := parseAPICall(resp)
		if apiReq == nil {
			// No api_call tag — LLM gave a final answer (already streamed if onChunk set).
			reply := stripAPICallTag(strings.TrimSpace(resp))
			a.memory.Add("user", userMessage)
			a.memory.Add("assistant", reply)
			return reply
		}

		// api_call iteration — reset display to thinking indicator before executing.
		if onChunk != nil {
			onChunk("⏳")
		}

		logger.Infof("Agent: iter=%d %s %s", i+1, apiReq.Method, apiReq.Path)
		result := a.apiTool.execute(apiReq)

		if textBefore != "" {
			turnMsgs = append(turnMsgs, mcp.NewAssistantMessage(textBefore))
		}
		turnMsgs = append(turnMsgs, mcp.NewUserMessage(
			fmt.Sprintf("[API result: %s %s]\n%s", apiReq.Method, apiReq.Path, result),
		))
	}

	// Safety: max iterations reached — ask LLM for a final summary (non-streaming).
	logger.Warnf("Agent: max iterations (%d) reached", maxIterations)
	turnMsgs = append(turnMsgs, mcp.NewUserMessage("Please summarize the results and give the user a final reply."))
	if finalReq, err := mcp.NewRequestBuilder().
		WithSystemPrompt(a.systemPrompt).
		AddConversationHistory(turnMsgs).
		Build(); err == nil {
		if finalResp, err := llm.CallWithRequest(finalReq); err == nil {
			lastResp = finalResp
		}
	}

	reply := stripAPICallTag(strings.TrimSpace(lastResp))
	a.memory.Add("user", userMessage)
	a.memory.Add("assistant", reply)
	return reply
}

// stripAPICallTag removes any <api_call>...</api_call> fragment from s.
// Used as a defensive layer to ensure tags never leak to the user.
func stripAPICallTag(s string) string {
	if idx := strings.Index(s, "<api_call>"); idx >= 0 {
		return strings.TrimSpace(s[:idx])
	}
	return s
}

// ResetMemory clears conversation history (called on /start).
func (a *Agent) ResetMemory() {
	a.memory.ResetFull()
}
