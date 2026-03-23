package api

import (
	"nofx/agent"

	"github.com/gin-gonic/gin"
)

// RegisterAgentHandler registers NOFXi agent API routes on the main router.
// Chat endpoint requires authentication; market data endpoints are public.
func (s *Server) RegisterAgentHandler(h *agent.WebHandler) {
	// Chat requires auth — can trigger trades and access account data
	s.router.POST("/api/agent/chat", s.authMiddleware(), gin.WrapF(h.HandleChat))
	// Public endpoints — read-only market data
	s.router.GET("/api/agent/health", gin.WrapF(h.HandleHealth))
	s.router.GET("/api/agent/klines", gin.WrapF(h.HandleKlines))
	s.router.GET("/api/agent/ticker", gin.WrapF(h.HandleTicker))
}
