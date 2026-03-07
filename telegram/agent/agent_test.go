package agent

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nofx/mcp"
)

type mockLLM struct {
	responses []string
	calls     int
	lastMsgs  []mcp.Message
}

func (m *mockLLM) SetAPIKey(_, _, _ string)                    {}
func (m *mockLLM) SetTimeout(_ time.Duration)                  {}
func (m *mockLLM) CallWithMessages(_, _ string) (string, error) { return m.next() }
func (m *mockLLM) CallWithRequest(req *mcp.Request) (string, error) {
	m.lastMsgs = req.Messages
	return m.next()
}
func (m *mockLLM) CallWithRequestStream(req *mcp.Request, onChunk func(string)) (string, error) {
	m.lastMsgs = req.Messages
	r, err := m.next()
	if onChunk != nil {
		onChunk(r)
	}
	return r, err
}
func (m *mockLLM) next() (string, error) {
	if m.calls < len(m.responses) {
		r := m.responses[m.calls]
		m.calls++
		return r, nil
	}
	return "OK", nil
}

func mockGetLLM(llm *mockLLM) func() mcp.AIClient {
	return func() mcp.AIClient { return llm }
}

const testPrompt = "You are a test assistant."

// TestAgentDirectReply: LLM replies without api_call — one call, direct reply.
func TestAgentDirectReply(t *testing.T) {
	llm := &mockLLM{responses: []string{"Hello! How can I help you?"}}
	a := New(8080, "tok", "test-user", mockGetLLM(llm), testPrompt)

	reply := a.Run("hello", nil)

	if reply != "Hello! How can I help you?" {
		t.Fatalf("unexpected reply: %q", reply)
	}
	if llm.calls != 1 {
		t.Fatalf("expected 1 LLM call, got %d", llm.calls)
	}
}

// TestAgentAPICall: LLM calls API, gets result, gives final reply — two LLM calls.
func TestAgentAPICall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/my-traders" {
			w.Write([]byte(`[{"id":"t1","name":"BTC Strategy"}]`)) //nolint:errcheck
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	var port int
	fmt.Sscanf(srv.Listener.Addr().String(), "127.0.0.1:%d", &port)

	llm := &mockLLM{responses: []string{
		`Let me check.<api_call>{"method":"GET","path":"/api/my-traders","body":{}}</api_call>`,
		"You have one trader: BTC Strategy.",
	}}
	a := New(port, "tok", "test-user", mockGetLLM(llm), testPrompt)

	reply := a.Run("list my traders", nil)

	if reply != "You have one trader: BTC Strategy." {
		t.Fatalf("unexpected reply: %q", reply)
	}
	if llm.calls != 2 {
		t.Fatalf("expected 2 LLM calls, got %d", llm.calls)
	}
}

// TestAgentMultiStep: LLM chains two API calls before final reply — three LLM calls.
func TestAgentMultiStep(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`)) //nolint:errcheck
	}))
	defer srv.Close()

	var port int
	fmt.Sscanf(srv.Listener.Addr().String(), "127.0.0.1:%d", &port)

	llm := &mockLLM{responses: []string{
		`Checking account.<api_call>{"method":"GET","path":"/api/account","body":{}}</api_call>`,
		`Now checking positions.<api_call>{"method":"GET","path":"/api/positions","body":{}}</api_call>`,
		"Account looks healthy and no open positions.",
	}}
	a := New(port, "tok", "test-user", mockGetLLM(llm), testPrompt)

	reply := a.Run("show me account status", nil)

	if llm.calls != 3 {
		t.Fatalf("expected 3 LLM calls (2 api + 1 final), got %d", llm.calls)
	}
	if reply != "Account looks healthy and no open positions." {
		t.Fatalf("unexpected final reply: %q", reply)
	}
}

// TestAgentAPIResultInContext: API result must appear in next LLM message.
func TestAgentAPIResultInContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"balance":1234.56}`)) //nolint:errcheck
	}))
	defer srv.Close()

	var port int
	fmt.Sscanf(srv.Listener.Addr().String(), "127.0.0.1:%d", &port)

	llm := &mockLLM{responses: []string{
		`<api_call>{"method":"GET","path":"/api/account","body":{}}</api_call>`,
		"Balance is 1234.56 USDT.",
	}}
	a := New(port, "tok", "test-user", mockGetLLM(llm), testPrompt)
	a.Run("show balance", nil)

	found := false
	for _, msg := range llm.lastMsgs {
		if strings.Contains(msg.Content, "API result") || strings.Contains(msg.Content, "balance") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("API result not found in subsequent LLM context")
	}
}

// TestParseAPICall: unit tests for the XML tag parser.
func TestParseAPICall(t *testing.T) {
	t.Run("valid call", func(t *testing.T) {
		resp := `Stopping trader.<api_call>{"method":"POST","path":"/api/traders/t1/stop","body":{}}</api_call>`
		req, text := parseAPICall(resp)
		if req == nil {
			t.Fatal("expected api_call, got nil")
		}
		if req.Method != "POST" || req.Path != "/api/traders/t1/stop" {
			t.Fatalf("unexpected req: %+v", req)
		}
		if text != "Stopping trader." {
			t.Fatalf("unexpected text before tag: %q", text)
		}
	})

	t.Run("no call tag", func(t *testing.T) {
		req, text := parseAPICall("Just a reply.")
		if req != nil {
			t.Fatal("expected nil api_call")
		}
		if text != "Just a reply." {
			t.Fatalf("expected original text, got %q", text)
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		req, _ := parseAPICall(`<api_call>NOT JSON</api_call>`)
		if req != nil {
			t.Fatal("expected nil for malformed JSON")
		}
	})
}
