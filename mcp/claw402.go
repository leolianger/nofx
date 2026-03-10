package mcp

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
)

const (
	ProviderClaw402    = "claw402"
	DefaultClaw402URL  = "https://claw402.ai"
	DefaultClaw402Model = "deepseek"
)

// claw402ModelEndpoints maps user-friendly model names to claw402 API paths.
var claw402ModelEndpoints = map[string]string{
	// OpenAI
	"gpt-5.4":     "/api/v1/ai/openai/chat/5.4",
	"gpt-5.4-pro": "/api/v1/ai/openai/chat/5.4-pro",
	"gpt-5.3":     "/api/v1/ai/openai/chat/5.3",
	"gpt-5-mini":  "/api/v1/ai/openai/chat/5-mini",
	// Anthropic
	"claude-opus": "/api/v1/ai/anthropic/messages/opus",
	// DeepSeek
	"deepseek":          "/api/v1/ai/deepseek/chat",
	"deepseek-reasoner": "/api/v1/ai/deepseek/chat/reasoner",
	// Qwen
	"qwen-max":   "/api/v1/ai/qwen/chat/max",
	"qwen-plus":  "/api/v1/ai/qwen/chat/plus",
	"qwen-turbo": "/api/v1/ai/qwen/chat/turbo",
	"qwen-flash": "/api/v1/ai/qwen/chat/flash",
	// Grok
	"grok-4.1": "/api/v1/ai/grok/chat/4.1",
	// Gemini
	"gemini-3.1-pro": "/api/v1/ai/gemini/chat/3.1-pro",
	// Kimi
	"kimi-k2.5": "/api/v1/ai/kimi/chat/k2.5",
}

// Claw402Client implements AIClient using claw402.ai's x402 v2 USDC payment gateway.
// Reuses the same EIP-712 signing as BlockRunBaseClient (same Base chain + USDC contract).
type Claw402Client struct {
	*Client
	privateKey *ecdsa.PrivateKey
}

// NewClaw402Client creates a claw402 client (backward compatible).
func NewClaw402Client() AIClient {
	return NewClaw402ClientWithOptions()
}

// NewClaw402ClientWithOptions creates a claw402 client with options.
func NewClaw402ClientWithOptions(opts ...ClientOption) AIClient {
	baseOpts := []ClientOption{
		WithProvider(ProviderClaw402),
		WithModel(DefaultClaw402Model),
		WithBaseURL(DefaultClaw402URL),
	}
	allOpts := append(baseOpts, opts...)
	baseClient := NewClient(allOpts...).(*Client)
	baseClient.UseFullURL = true
	baseClient.BaseURL = DefaultClaw402URL + claw402ModelEndpoints[DefaultClaw402Model]

	c := &Claw402Client{Client: baseClient}
	baseClient.hooks = c
	return c
}

// SetAPIKey stores the EVM private key and selects the model endpoint.
func (c *Claw402Client) SetAPIKey(apiKey string, _ string, customModel string) {
	hexKey := strings.TrimPrefix(apiKey, "0x")
	privKey, err := crypto.HexToECDSA(hexKey)
	if err != nil {
		c.logger.Warnf("⚠️  [MCP] Claw402: invalid private key: %v", err)
	} else {
		c.privateKey = privKey
		c.APIKey = apiKey
		addr := crypto.PubkeyToAddress(privKey.PublicKey).Hex()
		c.logger.Infof("🔧 [MCP] Claw402 wallet: %s", addr)
	}
	if customModel != "" {
		c.Model = customModel
	}
	endpoint := c.resolveEndpoint()
	c.BaseURL = DefaultClaw402URL + endpoint
	c.logger.Infof("🔧 [MCP] Claw402 model: %s → %s", c.Model, endpoint)
}

// resolveEndpoint returns the API path for the configured model.
func (c *Claw402Client) resolveEndpoint() string {
	if ep, ok := claw402ModelEndpoints[c.Model]; ok {
		return ep
	}
	// Allow raw path override (e.g. "/api/v1/ai/openai/chat/5.4")
	if strings.HasPrefix(c.Model, "/api/") {
		return c.Model
	}
	return claw402ModelEndpoints[DefaultClaw402Model]
}

func (c *Claw402Client) setAuthHeader(_ http.Header) {
	// No Bearer token — payment is via x402 signing
}

// call handles the x402 v2 payment flow for claw402.ai.
func (c *Claw402Client) call(systemPrompt, userPrompt string) (string, error) {
	c.logger.Infof("📡 [Claw402] Request AI: %s", c.BaseURL)

	requestBody := c.hooks.buildMCPRequestBody(systemPrompt, userPrompt)
	jsonData, err := c.hooks.marshalRequestBody(requestBody)
	if err != nil {
		return "", err
	}

	url := c.hooks.buildUrl()
	req, err := c.hooks.buildRequest(url, jsonData)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Handle x402 Payment Required
	if resp.StatusCode == http.StatusPaymentRequired {
		// Check both header variants
		paymentHeader := resp.Header.Get("X-Payment-Required")
		if paymentHeader == "" {
			paymentHeader = resp.Header.Get("Payment-Required")
		}
		if paymentHeader == "" {
			// Try reading payment details from response body
			body, _ := io.ReadAll(resp.Body)
			return "", fmt.Errorf("received 402 but no payment header found. Body: %s", string(body))
		}

		paymentSig, err := c.signPayment(paymentHeader)
		if err != nil {
			return "", fmt.Errorf("failed to sign x402 payment: %w", err)
		}

		req2, err := c.hooks.buildRequest(url, jsonData)
		if err != nil {
			return "", fmt.Errorf("failed to build retry request: %w", err)
		}
		// Send payment in both header variants for compatibility
		req2.Header.Set("X-Payment", paymentSig)
		req2.Header.Set("Payment-Signature", paymentSig)

		resp2, err := c.httpClient.Do(req2)
		if err != nil {
			return "", fmt.Errorf("failed to send payment retry: %w", err)
		}
		defer resp2.Body.Close()

		body2, err := io.ReadAll(resp2.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read payment response: %w", err)
		}
		if resp2.StatusCode != http.StatusOK {
			return "", fmt.Errorf("Claw402 payment retry failed (status %d): %s", resp2.StatusCode, string(body2))
		}
		return c.hooks.parseMCPResponse(body2)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Claw402 API error (status %d): %s", resp.StatusCode, string(body))
	}
	return c.hooks.parseMCPResponse(body)
}

// signPayment reuses the same EIP-712 signing logic as BlockRunBaseClient
// (same Base chain, same USDC contract, same TransferWithAuthorization).
func (c *Claw402Client) signPayment(paymentHeaderB64 string) (string, error) {
	if c.privateKey == nil {
		return "", fmt.Errorf("no private key set for Claw402 wallet")
	}

	// Decode base64 → JSON
	decoded, err := base64.RawStdEncoding.DecodeString(paymentHeaderB64)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(paymentHeaderB64)
		if err != nil {
			return "", fmt.Errorf("failed to decode payment header: %w", err)
		}
	}

	var req x402v2PaymentRequired
	if err := json.Unmarshal(decoded, &req); err != nil {
		return "", fmt.Errorf("failed to parse x402 v2 payment header: %w", err)
	}
	if len(req.Accepts) == 0 {
		return "", fmt.Errorf("no payment options in x402 response")
	}

	// Reuse the same signing logic as BlockRunBaseClient — identical chain + USDC contract
	opt := req.Accepts[0]
	senderAddr := crypto.PubkeyToAddress(c.privateKey.PublicKey).Hex()

	return signX402Payment(c.privateKey, senderAddr, opt, req.Resource)
}

// buildUrl returns the full claw402 endpoint URL.
func (c *Claw402Client) buildUrl() string {
	return c.BaseURL
}

// buildRequest creates the HTTP request without Authorization header.
func (c *Claw402Client) buildRequest(url string, jsonData []byte) (*http.Request, error) {
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("fail to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}
