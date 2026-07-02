package api

import (
	"net/http/httptest"
	"testing"

	"scriberr/internal/models"
	"scriberr/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestGlobalSettingsAndDiarizationPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:system-permissions-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}); err != nil {
		t.Fatalf("migrate users: %v", err)
	}

	userRepo := repository.NewUserRepository(db)
	owner := models.User{Username: "owner", Password: "hash"}
	member := models.User{Username: "member", Password: "hash"}
	if err := userRepo.Create(t.Context(), &owner); err != nil {
		t.Fatalf("create owner: %v", err)
	}
	if err := userRepo.Create(t.Context(), &member); err != nil {
		t.Fatalf("create member: %v", err)
	}

	handler := &Handler{userRepo: userRepo}
	ownerContext := authenticatedTestContext(owner.ID)
	memberContext := authenticatedTestContext(member.ID)
	multiUser := &models.SystemSetting{DeploymentMode: models.DeploymentModeMultiUser}
	singleUser := &models.SystemSetting{DeploymentMode: models.DeploymentModeSingleUser}

	if !handler.canManageGlobalSettings(ownerContext, multiUser) {
		t.Fatal("expected first registered user to manage global settings")
	}
	if handler.canManageGlobalSettings(memberContext, multiUser) {
		t.Fatal("expected non-owner to be denied global settings management")
	}
	if handler.canManageGlobalDiarization(memberContext, multiUser) {
		t.Fatal("expected non-owner to be denied resident-model controls in multi-user mode")
	}
	if !handler.canManageGlobalDiarization(memberContext, singleUser) {
		t.Fatal("expected resident-model controls to retain single-user behavior")
	}
}

func authenticatedTestContext(userID uint) *gin.Context {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("GET", "/", nil)
	context.Set("user_id", userID)
	return context
}
