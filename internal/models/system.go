package models

import (
	"strings"
	"time"
)

const (
	DeploymentModeSingleUser = "single_user"
	DeploymentModeMultiUser  = "multi_user"
)

// SystemSetting stores process-wide deployment and GPU residency settings.
// ID is fixed to 1 so the table remains a singleton.
type SystemSetting struct {
	ID                      uint      `json:"id" gorm:"primaryKey"`
	DeploymentMode          string    `json:"deployment_mode" gorm:"type:varchar(20);not null;default:'single_user'"`
	StartupDiarizationModel string    `json:"startup_diarization_model" gorm:"type:varchar(20);not null;default:'none'"`
	UpdatedAt               time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

func NormalizeDeploymentMode(value string) (string, bool) {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "", DeploymentModeSingleUser, "single-user", "single":
		return DeploymentModeSingleUser, true
	case DeploymentModeMultiUser, "multi-user", "multi":
		return DeploymentModeMultiUser, true
	default:
		return "", false
	}
}

func NormalizeStartupDiarizationModel(value string) (string, bool) {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "", "none":
		return "none", true
	case "pyannote", "pyannote/speaker-diarization-3.1", "pyannote/speaker-diarization-community-1":
		return "pyannote", true
	case "sortformer", "nvidia_sortformer", "nvidia/diar_streaming_sortformer_4spk-v2":
		return "sortformer", true
	default:
		return "", false
	}
}
