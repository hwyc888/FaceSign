package repository_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/hwyc888/FaceSign/internal/database"
	"github.com/hwyc888/FaceSign/internal/domain"
	"github.com/hwyc888/FaceSign/internal/repository"
)

func TestFaceSettingsPersistAndOverrideDefaults(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "facesign.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	repo := repository.New(db)
	defaults := domain.FaceSettings{
		Provider:           "disabled",
		ServiceURL:         "http://127.0.0.1:8000",
		Similarity:         0.78,
		DetectionThreshold: 0.80,
	}
	loaded, err := repo.LoadFaceSettings(context.Background(), defaults)
	if err != nil {
		t.Fatalf("load defaults: %v", err)
	}
	if loaded.Provider != defaults.Provider || loaded.ServiceURL != defaults.ServiceURL {
		t.Fatalf("defaults not returned: %#v", loaded)
	}

	want := domain.FaceSettings{
		Provider:           "compreface",
		ServiceURL:         "http://127.0.0.1:9000",
		APIKey:             "secret-key",
		Similarity:         0.82,
		DetectionThreshold: 0.75,
	}
	if err := repo.SaveFaceSettings(context.Background(), want); err != nil {
		t.Fatalf("save face settings: %v", err)
	}
	got, err := repo.LoadFaceSettings(context.Background(), defaults)
	if err != nil {
		t.Fatalf("reload face settings: %v", err)
	}
	if got != want {
		t.Fatalf("face settings = %#v, want %#v", got, want)
	}
}
