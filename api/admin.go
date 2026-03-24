package api

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

func superAdminEmail() string {
	if v := strings.TrimSpace(os.Getenv("NOFX_SUPER_ADMIN_EMAIL")); v != "" {
		return strings.ToLower(v)
	}
	if v := strings.TrimSpace(os.Getenv("NOFX_ADMIN_EMAIL")); v != "" {
		return strings.ToLower(v)
	}
	return "admin@example.com"
}

func (s *Server) handleAdminListUsers(c *gin.Context) {
	operatorEmail := strings.ToLower(strings.TrimSpace(c.GetString("email")))
	canManageRoles := operatorEmail == superAdminEmail()
	keyword := strings.ToLower(strings.TrimSpace(c.Query("q")))

	if keyword == "" {
		c.JSON(http.StatusOK, gin.H{
			"users":            []gin.H{},
			"can_manage_roles": canManageRoles,
		})
		return
	}

	users, err := s.store.User().List()
	if err != nil {
		SafeInternalError(c, "Failed to list users", err)
		return
	}

	superEmails := map[string]bool{superAdminEmail(): true, "admin@example.com": true}
	result := make([]gin.H, 0, len(users))
	for _, u := range users {
		email := strings.ToLower(strings.TrimSpace(u.Email))
		id := strings.ToLower(strings.TrimSpace(u.ID))
		role := strings.ToUpper(strings.TrimSpace(u.Role))

		if superEmails[email] || email == operatorEmail || role == "SUPER_ADMIN" {
			continue
		}
		if !strings.Contains(email, keyword) && !strings.Contains(id, keyword) {
			continue
		}

		result = append(result, gin.H{
			"id":    u.ID,
			"email": u.Email,
			"role":  role,
		})
	}

	sort.SliceStable(result, func(i, j int) bool {
		leftRole := result[i]["role"].(string)
		rightRole := result[j]["role"].(string)
		if leftRole == "ADMIN" && rightRole != "ADMIN" {
			return true
		}
		if rightRole == "ADMIN" && leftRole != "ADMIN" {
			return false
		}
		leftEmail := strings.ToLower(strings.TrimSpace(result[i]["email"].(string)))
		rightEmail := strings.ToLower(strings.TrimSpace(result[j]["email"].(string)))
		return leftEmail < rightEmail
	})

	c.JSON(http.StatusOK, gin.H{
		"users":            result,
		"can_manage_roles": canManageRoles,
	})
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

func (s *Server) handleAdminSetUserRole(c *gin.Context) {
	operatorEmail := strings.ToLower(strings.TrimSpace(c.GetString("email")))
	if operatorEmail != superAdminEmail() {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only super admin can manage admin roles"})
		return
	}

	userID := strings.TrimSpace(c.Param("id"))
	var req struct {
		Role string `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	targetRole := strings.ToUpper(strings.TrimSpace(req.Role))
	if targetRole != "ADMIN" && targetRole != "USER" {
		SafeBadRequest(c, "Role must be ADMIN or USER")
		return
	}

	user, err := s.store.User().GetByID(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	if strings.EqualFold(user.Email, superAdminEmail()) && targetRole != "ADMIN" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Cannot demote super admin"})
		return
	}

	if user.Role == "ADMIN" && targetRole != "ADMIN" {
		adminCount, err := s.store.User().CountByRole("ADMIN")
		if err != nil {
			SafeInternalError(c, "Failed to check admin count", err)
			return
		}
		if adminCount <= 1 {
			c.JSON(http.StatusForbidden, gin.H{"error": "Cannot demote the last admin"})
			return
		}
	}

	if err := s.store.User().UpdateRole(userID, targetRole); err != nil {
		SafeInternalError(c, "Failed to update user role", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User role updated"})
}

func (s *Server) handleAdminListStrategyWebhooks(c *gin.Context) {
	items, err := s.store.StrategyWebhook().List()
	if err != nil {
		SafeInternalError(c, "Failed to list strategy webhooks", err)
		return
	}

	strategies, err := s.store.Strategy().ListAll()
	if err != nil {
		SafeInternalError(c, "Failed to list strategies", err)
		return
	}
	strategyNameByID := make(map[string]string, len(strategies))
	for _, st := range strategies {
		if st == nil {
			continue
		}
		strategyNameByID[st.ID] = strings.TrimSpace(st.Name)
	}

	resp := make([]gin.H, 0, len(items))
	for _, it := range items {
		if it == nil {
			continue
		}
		resp = append(resp, gin.H{
			"strategy_id":   it.StrategyID,
			"strategy_name": strategyNameByID[it.StrategyID],
			"webhook_url":   it.WebhookURL,
			"enabled":       it.Enabled,
			"updated_at":    it.UpdatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": resp})
}

func validateDiscordWebhookURL(url string) error {
	value := strings.TrimSpace(url)
	if value == "" {
		return fmt.Errorf("webhook_url is required")
	}
	lower := strings.ToLower(value)
	if !strings.HasPrefix(lower, "https://discord.com/api/webhooks/") &&
		!strings.HasPrefix(lower, "https://discordapp.com/api/webhooks/") {
		return fmt.Errorf("webhook_url must be a Discord webhook URL")
	}
	return nil
}

func (s *Server) handleAdminUpsertStrategyWebhook(c *gin.Context) {
	var req struct {
		StrategyID string `json:"strategy_id" binding:"required"`
		WebhookURL string `json:"webhook_url" binding:"required"`
		Enabled    bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	req.StrategyID = strings.TrimSpace(req.StrategyID)
	if req.StrategyID == "" {
		SafeBadRequest(c, "strategy_id is required")
		return
	}
	if _, err := s.store.Strategy().GetByID(req.StrategyID); err != nil {
		SafeBadRequest(c, "Strategy not found")
		return
	}
	if err := validateDiscordWebhookURL(req.WebhookURL); err != nil {
		SafeBadRequest(c, err.Error())
		return
	}
	if err := s.store.StrategyWebhook().Upsert(req.StrategyID, req.WebhookURL, req.Enabled); err != nil {
		SafeInternalError(c, "Failed to save strategy webhook", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Strategy webhook updated"})
}

func (s *Server) handleAdminDeleteStrategyWebhook(c *gin.Context) {
	strategyID := strings.TrimSpace(c.Param("id"))
	if strategyID == "" {
		SafeBadRequest(c, "strategy_id is required")
		return
	}
	if err := s.store.StrategyWebhook().Delete(strategyID); err != nil {
		SafeInternalError(c, "Failed to delete strategy webhook", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Strategy webhook deleted"})
}

func (s *Server) handleAdminListSignalNotifyUsers(c *gin.Context) {
	items, err := s.store.SignalNotifyUser().List()
	if err != nil {
		SafeInternalError(c, "Failed to list signal notify users", err)
		return
	}
	resp := make([]gin.H, 0, len(items))
	for _, it := range items {
		if it == nil {
			continue
		}
		resp = append(resp, gin.H{
			"email":      it.Email,
			"enabled":    it.Enabled,
			"updated_at": it.UpdatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": resp})
}

func (s *Server) handleAdminUpsertSignalNotifyUser(c *gin.Context) {
	var req struct {
		Email   string `json:"email" binding:"required"`
		Enabled bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || !strings.Contains(email, "@") {
		SafeBadRequest(c, "email is invalid")
		return
	}
	if err := s.store.SignalNotifyUser().Upsert(email, req.Enabled); err != nil {
		SafeInternalError(c, "Failed to save notify user", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Notify user updated"})
}

func (s *Server) handleAdminDeleteSignalNotifyUser(c *gin.Context) {
	email := strings.ToLower(strings.TrimSpace(c.Param("email")))
	if email == "" {
		SafeBadRequest(c, "email is required")
		return
	}
	if err := s.store.SignalNotifyUser().Delete(email); err != nil {
		SafeInternalError(c, "Failed to delete notify user", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Notify user deleted"})
}
