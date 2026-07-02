package repository

import (
	"testing"

	"scriberr/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestSystemSettingsGetOrCreateMigratesLegacyStartupModelOnce(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:system-settings-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.AutoMigrate(&models.SystemSetting{}); err != nil {
		t.Fatalf("migrate system settings: %v", err)
	}

	repo := NewSystemSettingsRepository(db)
	settings, err := repo.GetOrCreate(t.Context(), "pyannote")
	if err != nil {
		t.Fatalf("create settings: %v", err)
	}
	if settings.DeploymentMode != models.DeploymentModeSingleUser || settings.StartupDiarizationModel != "pyannote" {
		t.Fatalf("unexpected migrated settings: %#v", settings)
	}

	settings, err = repo.GetOrCreate(t.Context(), "sortformer")
	if err != nil {
		t.Fatalf("reload settings: %v", err)
	}
	if settings.StartupDiarizationModel != "pyannote" {
		t.Fatalf("expected persisted global startup model, got %q", settings.StartupDiarizationModel)
	}
}
