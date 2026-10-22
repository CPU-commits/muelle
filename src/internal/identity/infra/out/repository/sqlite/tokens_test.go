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
		proj.StageSlug != "" || proj.StageID != nil || !proj.ExpiresAt.Equal(exp) || *proj.CreatedBy != u.ID || proj.CreatedByName != "felipe" ||
		!proj.CreatedAt.Equal(t0) {
		t.Fatalf("proj=%+v err=%v", proj, err)
	}
	stg, err := st.Create(ctx, entity.APIToken{Name: "ci-staging", TokenHash: "h2", ProjectID: projectID, StageID: &stageID, CreatedAt: t0})
	if err != nil || stg.StageSlug != "staging" || stg.StageID == nil || *stg.StageID != stageID || stg.ExpiresAt != nil || stg.CreatedBy != nil || stg.CreatedByName != "" {
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

// Un token no se borra con quien lo creó: es del proyecto, y el CI depende de
// él. Queda sin creador (created_by NULL), y sigue sirviendo para autenticar.
func TestTokenOutlivesItsCreator(t *testing.T) {
	r := newRepos(t)
	ctx := context.Background()
	projectID, _ := seedStage(t, r.db)
	ana := r.user(t, "ana", nil)
	tok, err := r.tokens.Create(ctx, entity.APIToken{Name: "ci", TokenHash: "h1", ProjectID: projectID, CreatedBy: &ana.ID, CreatedAt: t0})
	if err != nil || tok.CreatedByName != "ana" {
		t.Fatalf("tok=%+v err=%v", tok, err)
	}
	if err := r.users.Delete(ctx, ana.ID); err != nil {
		t.Fatal(err)
	}
	got, err := r.tokens.GetByHash(ctx, "h1")
	if err != nil || got.ID != tok.ID || got.CreatedBy != nil || got.CreatedByName != "" {
		t.Fatalf("después de borrar a ana: got=%+v err=%v", got, err)
	}
	if list, err := r.tokens.List(ctx); err != nil || len(list) != 1 || list[0].CreatedBy != nil || list[0].CreatedByName != "" {
		t.Fatalf("list=%+v err=%v", list, err)
	}
}

// Un token de stage cuyo stage ya no existe (borrado con foreign_keys=OFF) o
// es de otro proyecto no puede convertirse en un token de todo el proyecto:
// el repositorio no lo devuelve. Lo mismo un token cuyo proyecto no existe.
func TestTokensWithBrokenScope(t *testing.T) {
	r, path := newReposAt(t)
	ctx := context.Background()
	projectID, stageID := seedStage(t, r.db)
	otherID := exec(t, r.db, `INSERT INTO projects (slug, name, created_at) VALUES ('otro', 'Otro', 0)`)
	movedID := exec(t, r.db, `INSERT INTO stages (project_id, slug, created_at) VALUES (?, 'production', 0)`, projectID)
	orphan, _ := r.tokens.Create(ctx, entity.APIToken{Name: "huerfano", TokenHash: "h1", ProjectID: projectID, StageID: &stageID, CreatedAt: t0})
	moved, _ := r.tokens.Create(ctx, entity.APIToken{Name: "movido", TokenHash: "h2", ProjectID: projectID, StageID: &movedID, CreatedAt: t0})
	whole, _ := r.tokens.Create(ctx, entity.APIToken{Name: "proyecto", TokenHash: "h3", ProjectID: projectID, CreatedAt: t0})
	other, _ := r.tokens.Create(ctx, entity.APIToken{Name: "otro", TokenHash: "h4", ProjectID: otherID, CreatedAt: t0})

	rawExec(t, path, `DELETE FROM stages WHERE id = ?`, stageID)
	rawExec(t, path, `UPDATE stages SET project_id = ? WHERE id = ?`, otherID, movedID)
	rawExec(t, path, `DELETE FROM projects WHERE id = ?`, otherID)

	for _, hash := range []string{"h1", "h2", "h4"} {
		if tok, err := r.tokens.GetByHash(ctx, hash); !errors.Is(err, repository.ErrNotFound) {
			t.Errorf("%s: esperaba ErrNotFound, got tok=%+v err=%v", hash, tok, err)
		}
	}
	if tok, err := r.tokens.GetByHash(ctx, "h3"); err != nil || tok.StageSlug != "" || tok.ProjectSlug != "auditfy" {
		t.Fatalf("el token de proyecto sigue sirviendo: tok=%+v err=%v", tok, err)
	}
	// La lista no los muestra como si fueran de todo el proyecto.
	list, err := r.tokens.List(ctx)
	if err != nil || len(list) != 1 || list[0].ID != whole.ID {
		t.Fatalf("list=%+v err=%v (huérfano %d, movido %d, otro %d)", list, err, orphan.ID, moved.ID, other.ID)
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

// Los tres alcances con las dos marcas: de proyecto, de stage y de todos los
// proyectos (sin proyecto ni stage, siempre de solo lectura).
func TestTokensReadOnlyAndAllProjects(t *testing.T) {
	r := newRepos(t)
	ctx := context.Background()
	projectID, stageID := seedStage(t, r.db)
	u := r.user(t, "felipe", nil)

	proj, err := r.tokens.Create(ctx, entity.APIToken{Name: "ro", TokenHash: "h1", ProjectID: projectID, ReadOnly: true, CreatedBy: &u.ID, CreatedAt: t0})
	if err != nil || !proj.ReadOnly || proj.AllProjects || proj.ProjectSlug != "auditfy" || proj.StageSlug != "" || !proj.ScopeValid() {
		t.Fatalf("proj=%+v err=%v", proj, err)
	}
	stg, err := r.tokens.Create(ctx, entity.APIToken{Name: "ro-staging", TokenHash: "h2", ProjectID: projectID, StageID: &stageID, ReadOnly: true, CreatedAt: t0})
	if err != nil || !stg.ReadOnly || stg.AllProjects || stg.StageSlug != "staging" || !stg.ScopeValid() {
		t.Fatalf("stg=%+v err=%v", stg, err)
	}
	all, err := r.tokens.Create(ctx, entity.APIToken{Name: "todos", TokenHash: "h3", ReadOnly: true, AllProjects: true, CreatedBy: &u.ID, CreatedAt: t0})
	if err != nil || !all.ReadOnly || !all.AllProjects || all.ProjectID != 0 || all.ProjectSlug != "" ||
		all.StageID != nil || all.StageSlug != "" || all.CreatedByName != "felipe" || !all.ScopeValid() {
		t.Fatalf("all=%+v err=%v", all, err)
	}
	write, err := r.tokens.Create(ctx, entity.APIToken{Name: "rw", TokenHash: "h4", ProjectID: projectID, CreatedAt: t0})
	if err != nil || write.ReadOnly || write.AllProjects {
		t.Fatalf("write=%+v err=%v", write, err)
	}

	got, err := r.tokens.GetByHash(ctx, "h3")
	if err != nil || got.ID != all.ID || !got.AllProjects || !got.ReadOnly || got.ProjectSlug != "" {
		t.Fatalf("GetByHash de todos los proyectos: got=%+v err=%v", got, err)
	}
	got, err = r.tokens.GetByHash(ctx, "h2")
	if err != nil || !got.ReadOnly || got.AllProjects || got.StageSlug != "staging" {
		t.Fatalf("GetByHash de stage: got=%+v err=%v", got, err)
	}
	list, err := r.tokens.List(ctx)
	if err != nil || len(list) != 4 || list[2].ID != all.ID || !list[2].AllProjects || !list[0].ReadOnly || list[3].ReadOnly {
		t.Fatalf("list=%+v err=%v", list, err)
	}

	// La base no deja un token de escritura de todos los proyectos.
	if _, err := r.tokens.Create(ctx, entity.APIToken{Name: "x", TokenHash: "h5", AllProjects: true, CreatedAt: t0}); err == nil {
		t.Fatal("la base aceptó un token de escritura de todos los proyectos")
	}

	// Un token de todos los proyectos sigue aunque se borre el proyecto.
	exec(t, r.db, `DELETE FROM api_tokens WHERE project_id = ?`, projectID)
	exec(t, r.db, `DELETE FROM stages WHERE id = ?`, stageID)
	exec(t, r.db, `DELETE FROM projects WHERE id = ?`, projectID)
	if got, err := r.tokens.GetByHash(ctx, "h3"); err != nil || got.ID != all.ID {
		t.Fatalf("todos los proyectos sin proyectos: got=%+v err=%v", got, err)
	}
}

// Un token de proyecto cuyo proyecto desapareció (borrado con
// foreign_keys=OFF) nunca se lee como uno de todos los proyectos: con el
// LEFT JOIN su proyecto llega NULL, y el repositorio no lo devuelve.
func TestTokenWithoutItsProjectIsNotAllProjects(t *testing.T) {
	r, path := newReposAt(t)
	ctx := context.Background()
	projectID := exec(t, r.db, `INSERT INTO projects (slug, name, created_at) VALUES ('auditfy', 'Auditfy', 0)`)
	if _, err := r.tokens.Create(ctx, entity.APIToken{Name: "ro", TokenHash: "h1", ProjectID: projectID, ReadOnly: true, CreatedAt: t0}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.tokens.Create(ctx, entity.APIToken{Name: "rw", TokenHash: "h2", ProjectID: projectID, CreatedAt: t0}); err != nil {
		t.Fatal(err)
	}
	all, err := r.tokens.Create(ctx, entity.APIToken{Name: "todos", TokenHash: "h3", ReadOnly: true, AllProjects: true, CreatedAt: t0})
	if err != nil {
		t.Fatal(err)
	}
	rawExec(t, path, `DELETE FROM projects WHERE id = ?`, projectID)

	for _, hash := range []string{"h1", "h2"} {
		if tok, err := r.tokens.GetByHash(ctx, hash); !errors.Is(err, repository.ErrNotFound) {
			t.Errorf("%s: esperaba ErrNotFound, got tok=%+v err=%v", hash, tok, err)
		}
	}
	list, err := r.tokens.List(ctx)
	if err != nil || len(list) != 1 || list[0].ID != all.ID {
		t.Fatalf("list=%+v err=%v", list, err)
	}
}
