package api

import (
	"nofx/agent"

	"github.com/gin-gonic/gin"
)

// RegisterAgentHandler registers NOFXi agent API routes on the main router.
func (s *Server) RegisterAgentHandler(h *agent.WebHandler) {
	s.router.POST("/api/agent/chat", gin.WrapF(h.HandleChat))
	s.router.GET("/api/agent/health", gin.WrapF(h.HandleHealth))
	s.router.GET("/api/agent/klines", gin.WrapF(h.HandleKlines))
	s.router.GET("/api/agent/ticker", gin.WrapF(h.HandleTicker))
}
