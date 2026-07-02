package main

import (
	"context"
	"os"
	"testing"

	"scriberr/internal/models"
	"scriberr/internal/repository"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestApplyStartupProfileHFTokenKeepsTokenRequestScoped(t *testing.T) {
	t.Setenv("HF_TOKEN", "")
	db, err := gorm.Open(sqlite.Open("file:startup-token-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.AutoMigrate(&models.TranscriptionProfile{}); err != nil {
		t.Fatalf("migrate profile: %v", err)
	}

	token := "profile-scoped-token"
	profile := models.TranscriptionProfile{
		ID:   "profile-1",
		Name: "Default",
		Parameters: models.WhisperXParams{
			HfToken: &token,
		},
	}
	profileRepo := repository.NewProfileRepository(db)
	if err := profileRepo.Create(context.Background(), &profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	params := map[string]interface{}{}
	profileID := profile.ID
	applyStartupProfileHFToken(context.Background(), 1, &profileID, profileRepo, params)

	if got := params["hf_token"]; got != token {
		t.Fatalf("expected scoped token in load params, got %#v", got)
	}
	if got := os.Getenv("HF_TOKEN"); got != "" {
		t.Fatalf("expected process HF_TOKEN to remain unset, got %q", got)
	}
}
