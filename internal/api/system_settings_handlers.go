package api

import (
	"net/http"

	"scriberr/internal/models"

	"github.com/gin-gonic/gin"
)

type SystemSettingsResponse struct {
	DeploymentMode          string `json:"deployment_mode"`
	StartupDiarizationModel string `json:"startup_diarization_model"`
	CanManage               bool   `json:"can_manage"`
}

type UpdateSystemSettingsRequest struct {
	DeploymentMode          *string `json:"deployment_mode,omitempty"`
	StartupDiarizationModel *string `json:"startup_diarization_model,omitempty"`
}

func (h *Handler) GetSystemSettings(c *gin.Context) {
	settings, err := h.systemSettingsRepo.GetOrCreate(c.Request.Context(), "none")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load system settings"})
		return
	}

	c.JSON(http.StatusOK, h.systemSettingsResponse(c, settings))
}

func (h *Handler) UpdateSystemSettings(c *gin.Context) {
	settings, err := h.systemSettingsRepo.GetOrCreate(c.Request.Context(), "none")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load system settings"})
		return
	}
	if !h.canManageGlobalSettings(c, settings) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only the deployment owner can change global settings"})
		return
	}

	var req UpdateSystemSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	if req.DeploymentMode != nil {
		deploymentMode, ok := models.NormalizeDeploymentMode(*req.DeploymentMode)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "deployment_mode must be one of: single_user, multi_user"})
			return
		}
		settings.DeploymentMode = deploymentMode
	}
	if req.StartupDiarizationModel != nil {
		startupModel, ok := models.NormalizeStartupDiarizationModel(*req.StartupDiarizationModel)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "startup_diarization_model must be one of: none, pyannote, sortformer"})
			return
		}
		settings.StartupDiarizationModel = startupModel
	}

	if err := h.systemSettingsRepo.Save(c.Request.Context(), settings); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save system settings"})
		return
	}

	c.JSON(http.StatusOK, h.systemSettingsResponse(c, settings))
}

func (h *Handler) systemSettingsResponse(c *gin.Context, settings *models.SystemSetting) SystemSettingsResponse {
	return SystemSettingsResponse{
		DeploymentMode:          settings.DeploymentMode,
		StartupDiarizationModel: settings.StartupDiarizationModel,
		CanManage:               h.canManageGlobalSettings(c, settings),
	}
}

func (h *Handler) canManageGlobalSettings(c *gin.Context, settings *models.SystemSetting) bool {
	userID, exists := c.Get("user_id")
	if !exists {
		return false
	}
	owner, err := h.userRepo.FindFirst(c.Request.Context())
	if err != nil {
		return false
	}
	id, ok := userID.(uint)
	return ok && id == owner.ID
}

func (h *Handler) canManageGlobalDiarization(c *gin.Context, settings *models.SystemSetting) bool {
	if settings.DeploymentMode != models.DeploymentModeMultiUser {
		return true
	}
	return h.canManageGlobalSettings(c, settings)
}

func (h *Handler) requireGlobalDiarizationControl(c *gin.Context) bool {
	settings, err := h.systemSettingsRepo.GetOrCreate(c.Request.Context(), "none")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load system settings"})
		return false
	}
	if h.canManageGlobalDiarization(c, settings) {
		return true
	}

	c.JSON(http.StatusForbidden, gin.H{"error": "Only the deployment owner can manage the resident diarization model in multi-user mode"})
	return false
}
