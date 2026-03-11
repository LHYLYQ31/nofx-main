package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func (s *Server) handleAdminListUsers(c *gin.Context) {
	users, err := s.store.User().List()
	if err != nil {
		SafeInternalError(c, "Failed to list users", err)
		return
	}

	result := make([]gin.H, 0, len(users))
	for _, u := range users {
		result = append(result, gin.H{
			"id":    u.ID,
			"email": u.Email,
			"role":  u.Role,
		})
	}
	c.JSON(http.StatusOK, gin.H{"users": result})
}

func (s *Server) handleAdminGetUserStrategies(c *gin.Context) {
	userID := strings.TrimSpace(c.Param("id"))
	ids, err := s.store.UserStrategyPermission().ListStrategyIDsByUser(userID)
	if err != nil {
		SafeInternalError(c, "Failed to get user strategies", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"strategy_ids": ids})
}

func (s *Server) handleAdminSetUserStrategies(c *gin.Context) {
	userID := strings.TrimSpace(c.Param("id"))
	var req struct {
		StrategyIDs []string `json:"strategy_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}
	if err := s.store.UserStrategyPermission().ReplaceUserStrategies(userID, req.StrategyIDs); err != nil {
		SafeInternalError(c, "Failed to update user strategy permissions", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "User strategy permissions updated"})
}
