package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/repository"
)

func TestTokens(t *testing.T) {
	r := newRepos(t)
	ctx := context.Background()
	st := r.tokens
	projectID, stageID := seedStage(t, r.db)
	u := r.user(t, "felipe", nil)

	exp := t0.Add(24 * time.Hour)
	proj, err := st.Create(ctx, entity.APIToken{Name: "ci-todo", TokenHash: "h1", ProjectID: projectID, CreatedBy: &u.ID, CreatedAt: t0, ExpiresAt: &exp})
	if err != nil || proj.ID == 0 || proj.Name != "ci-todo" || proj.ProjectSlug != "auditfy" || proj.ProjectID != projectID ||
		proj.StageSlug != "" || proj.StageID != nil || !proj.ExpiresAt.Equal(exp) || *proj.CreatedBy != u.ID || !proj.CreatedAt.Equal(t0) {
		t.Fatalf("proj=%+v err=%v", proj, err)
	}
	stg, err := st.Create(ctx, entity.APIToken{Name: "ci-staging", TokenHash: "h2", ProjectID: projectID, StageID: &stageID, CreatedAt: t0})
	if err != nil || stg.StageSlug != "staging" || stg.StageID == nil || *stg.StageID != stageID || stg.ExpiresAt != nil || stg.CreatedBy != nil {
		t.Fatalf("stg=%+v err=%v", stg, err)
	}
	missing := int64(999)
	if _, err := st.Create(ctx, entity.APIToken{Name: "x", TokenHash: "h3", ProjectID: projectID, StageID: &missing, CreatedAt: t0}); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("stage inexistente: esperaba ErrNotFound, got %v", err)
	}
	if _, err := st.Create(ctx, entity.APIToken{Name: "x", TokenHash: "h3", ProjectID: missing, CreatedAt: t0}); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("proyecto inexistente: esperaba ErrNotFound, got %v", err)
	}
	if _, err := st.Create(ctx, entity.APIToken{Name: "x", TokenHash: "h1", ProjectID: projectID, CreatedAt: t0}); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("hash repetido: esperaba ErrConflict, got %v", err)
	}

	got, err := st.GetByHash(ctx, "h2")
	if err != nil || got.ID != stg.ID || got.LastUsedAt != nil || got.StageSlug != "staging" || got.ProjectSlug != "auditfy" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if _, err := st.GetByHash(ctx, "no-existe"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}
	if err := st.Touch(ctx, stg.ID, t0); err != nil {
		t.Fatal(err)
	}
	got, _ = st.GetByHash(ctx, "h2")
	if got.LastUsedAt == nil || !got.LastUsedAt.Equal(t0) {
		t.Fatalf("last_used=%v", got.LastUsedAt)
	}

	list, _ := st.List(ctx)
	if len(list) != 2 || list[0].ID != proj.ID || list[1].ID != stg.ID {
		t.Fatalf("list=%+v", list)
	}
	if err := st.Delete(ctx, proj.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.Delete(ctx, proj.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}

	// Borrar el stage borra sus tokens.
	exec(t, r.db, `DELETE FROM stages WHERE id = ?`, stageID)
	if _, err := st.GetByHash(ctx, "h2"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("el token del stage debió borrarse, got %v", err)
	}
}

func TestTokensCascadeWithProject(t *testing.T) {
	r := newRepos(t)
	ctx := context.Background()
	projectID := exec(t, r.db, `INSERT INTO projects (slug, name, created_at) VALUES ('auditfy', 'Auditfy', 0)`)
	if _, err := r.tokens.Create(ctx, entity.APIToken{Name: "ci", TokenHash: "h1", ProjectID: projectID, CreatedAt: t0}); err != nil {
		t.Fatal(err)
	}
	exec(t, r.db, `DELETE FROM projects WHERE id = ?`, projectID)
	if list, _ := r.tokens.List(ctx); len(list) != 0 {
		t.Fatalf("el token debió borrarse con su proyecto: %+v", list)
	}
}

func TestDeleteUserKeepsTokens(t *testing.T) {
	r := newRepos(t)
	ctx := context.Background()
	projectID, _ := seedStage(t, r.db)
	u := r.user(t, "felipe", nil)
	r.tokens.Create(ctx, entity.APIToken{Name: "ci", TokenHash: "h1", ProjectID: projectID, CreatedBy: &u.ID, CreatedAt: t0})
	r.users.Delete(ctx, u.ID)
	tok, err := r.tokens.GetByHash(ctx, "h1")
	if err != nil || tok.CreatedBy != nil {
		t.Fatalf("el token sigue, sin creador: tok=%+v err=%v", tok, err)
	}
}
