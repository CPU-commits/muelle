package usecases

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

func newToken(name, project, stage string, createdBy int64, days int) dto.CreateTokenInput {
	return dto.CreateTokenInput{Name: name, Project: project, Stage: stage, CreatedBy: createdBy, ExpiresInDays: days}
}

func TestAPITokens(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")
	auditfy := f.project(t, "auditfy")
	f.stage(t, auditfy, "staging")

	tok, err := f.createToken.Execute(ctx, newToken("ci", "auditfy", "staging", u.ID, 30))
	if err != nil || tok.Stage != "staging" || tok.Project != "auditfy" || tok.Name != "ci" || tok.ExpiresAt == nil ||
		!tok.ExpiresAt.Equal(f.now().AddDate(0, 0, 30)) || !strings.HasPrefix(tok.Plain, "mk_") || len(tok.Plain) != 46 {
		t.Fatalf("tok=%+v err=%v", tok, err)
	}
	p, err := f.authToken.Execute(ctx, tok.Plain)
	if err != nil || !p.IsToken() || p.Token.ProjectSlug != "auditfy" || p.Token.StageSlug != "staging" || p.Token.Name != "ci" {
		t.Fatalf("p=%+v err=%v", p, err)
	}
	kind, id := p.Actor()
	if kind != shared.ActorToken || *id != tok.ID || p.ActorName() != "ci" {
		t.Fatalf("actor=%s %v %q", kind, id, p.ActorName())
	}
	if _, err := f.authToken.Execute(ctx, "mk_inventado"); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("got %v", err)
	}
	if _, err := f.authToken.Execute(ctx, "sin-prefijo"); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("got %v", err)
	}
	f.advance(31 * 24 * time.Hour)
	if _, err := f.authToken.Execute(ctx, tok.Plain); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("token vencido: got %v", err)
	}
	if _, err := f.createToken.Execute(ctx, newToken("", "auditfy", "", u.ID, 0)); err == nil {
		t.Fatal("nombre vacío debió fallar")
	}
}

func TestCreateTokenValidation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")
	f.project(t, "auditfy")
	otro := f.project(t, "otro")
	f.stage(t, otro, "staging")

	for _, days := range []int{-1, service.MaxTokenDays + 1} {
		_, err := f.createToken.Execute(ctx, newToken("ci", "auditfy", "", u.ID, days))
		if ve, ok := errors.AsType[*validate.Error](err); !ok || ve.Field != "expires_in_days" {
			t.Errorf("%d días: esperaba un error de expires_in_days, got %v", days, err)
		}
	}
	for _, c := range [][2]string{{"nope", ""}, {"", ""}, {"auditfy", "nope"}, {"auditfy", "staging"}} {
		if _, err := f.createToken.Execute(ctx, newToken("ci", c[0], c[1], u.ID, 0)); !errors.Is(err, sharedrepo.ErrNotFound) {
			t.Errorf("%s/%s: esperaba ErrNotFound, got %v", c[0], c[1], err)
		}
	}
	tok, err := f.createToken.Execute(ctx, newToken("ci", "auditfy", "", u.ID, service.MaxTokenDays))
	if err != nil || tok.Stage != "" || tok.ExpiresAt == nil {
		t.Fatalf("tok=%+v err=%v", tok, err)
	}
	if tok, err := f.createToken.Execute(ctx, newToken("ci", "auditfy", "", u.ID, 0)); err != nil || tok.ExpiresAt != nil {
		t.Fatalf("0 días es que no vence: tok=%+v err=%v", tok, err)
	}
	if len(f.auditRows(t)) != 2 {
		t.Fatalf("solo los tokens creados quedan en la auditoría: %+v", f.auditRows(t))
	}
}

func TestAuthenticateTokenTouchesOncePerMinute(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")
	f.project(t, "auditfy")
	tok, _ := f.createToken.Execute(ctx, newToken("ci", "auditfy", "", u.ID, 0))
	if tok.LastUsedAt != nil {
		t.Fatalf("un token nuevo no se usó: %v", tok.LastUsedAt)
	}

	p, err := f.authToken.Execute(ctx, tok.Plain)
	if err != nil || p.Token.StageSlug != "" || p.Token.ProjectSlug != "auditfy" || p.Token.ID != tok.ID {
		t.Fatalf("un token de proyecto llena el alcance sin stage: p=%+v err=%v", p, err)
	}
	first := f.now()
	f.advance(30 * time.Second)
	f.authToken.Execute(ctx, tok.Plain)
	list, _ := f.listTokens.Execute(ctx)
	if list[0].LastUsedAt == nil || !list[0].LastUsedAt.Equal(first) {
		t.Fatalf("last_used=%v, esperaba %v", list[0].LastUsedAt, first)
	}
	f.advance(30 * time.Second)
	f.authToken.Execute(ctx, tok.Plain)
	list, _ = f.listTokens.Execute(ctx)
	if !list[0].LastUsedAt.Equal(f.now()) {
		t.Fatalf("last_used=%v, esperaba %v", list[0].LastUsedAt, f.now())
	}
}

func TestTokenAuditAndDelete(t *testing.T) {
	f := newFixture(t)
	u := f.user(t, "felipe")
	ctx := withUser(u)
	auditfy := f.project(t, "auditfy")
	f.stage(t, auditfy, "staging")

	tok, err := f.createToken.Execute(ctx, newToken("ci", "auditfy", "staging", u.ID, 0))
	if err != nil {
		t.Fatal(err)
	}
	list, _ := f.listTokens.Execute(ctx)
	if len(list) != 1 || list[0].ID != tok.ID || list[0].Stage != "staging" {
		t.Fatalf("list=%+v", list)
	}
	if err := f.deleteToken.Execute(ctx, tok.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.deleteToken.Execute(ctx, tok.ID); !errors.Is(err, sharedrepo.ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}
	// Revocar un token lo invalida.
	if _, err := f.authToken.Execute(ctx, tok.Plain); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("token revocado: got %v", err)
	}

	rows := f.auditRows(t)
	target := "token:" + itoa(tok.ID)
	if len(rows) != 2 ||
		rows[0].Action != "token.create" || rows[0].Target != target || rows[0].Details != `{"project":"auditfy","stage":"staging"}` ||
		rows[1].Action != "token.delete" || rows[1].Target != target || *rows[1].ActorID != u.ID {
		t.Fatalf("audit=%+v", rows)
	}
}

// TestTokenScopes es la matriz de permisos de §10.1 (Tarea 22 de la
// referencia) en lo que depende de identity: AuthenticateToken produce el
// alcance y Principal.Allows decide. Las filas que dependen del nivel de
// acceso de cada ruta (un token en una ruta de solo sesión da 403
// token_not_allowed) se prueban en HTTP y en la integración.
func TestTokenScopes(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")
	auditfy := f.project(t, "auditfy")
	f.stage(t, auditfy, "staging")
	f.stage(t, auditfy, "production")
	f.project(t, "otro")

	auth := func(stage string) shared.Principal {
		t.Helper()
		tok, err := f.createToken.Execute(ctx, newToken("ci", "auditfy", stage, u.ID, 0))
		if err != nil {
			t.Fatal(err)
		}
		p, err := f.authToken.Execute(ctx, tok.Plain)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	stagingTok := auth("staging")
	projectTok := auth("")

	cases := []struct {
		name    string
		p       shared.Principal
		project string
		stage   string
		write   bool
		want    bool
	}{
		{"stage: lee su app", stagingTok, "auditfy", "staging", false, true},
		{"stage: reemplaza env de su app o su stage", stagingTok, "auditfy", "staging", true, true},
		{"stage: lee el proyecto", stagingTok, "auditfy", "", false, true},
		{"stage: no escribe en el proyecto", stagingTok, "auditfy", "", true, false},
		{"stage: no toca otro stage", stagingTok, "auditfy", "production", true, false},
		{"stage: no lee otro stage", stagingTok, "auditfy", "production", false, false},
		{"stage: no toca otro proyecto", stagingTok, "otro", "", false, false},
		{"proyecto: toca cualquier stage", projectTok, "auditfy", "production", true, true},
		{"proyecto: lee el proyecto", projectTok, "auditfy", "", false, true},
		{"proyecto: no toca otro proyecto", projectTok, "otro", "", false, false},
	}
	for _, c := range cases {
		if got := c.p.Allows(c.project, c.stage, c.write); got != c.want {
			t.Errorf("%s: Allows=%v, esperaba %v", c.name, got, c.want)
		}
	}

	if _, err := f.authToken.Execute(ctx, "mk_inventado"); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("token inventado: got %v", err)
	}
	// Revocar un token lo invalida.
	list, _ := f.listTokens.Execute(ctx)
	if err := f.deleteToken.Execute(ctx, list[0].ID); err != nil {
		t.Fatal(err)
	}
	remaining, _ := f.listTokens.Execute(ctx)
	if len(remaining) != 1 || remaining[0].ID == list[0].ID {
		t.Fatalf("remaining=%+v", remaining)
	}
}

// Quien pide el token fue borrado mientras su petición estaba en curso: 401,
// no un 404 que haría pensar que falta el proyecto.
func TestCreateTokenByDeletedUser(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	gone := f.user(t, "ana")
	f.project(t, "auditfy")
	f.users.Delete(ctx, gone.ID)
	if _, err := f.createToken.Execute(ctx, newToken("ci", "auditfy", "", gone.ID, 0)); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("esperaba ErrUnauthenticated, got %v", err)
	}
	if list, _ := f.listTokens.Execute(ctx); len(list) != 0 {
		t.Fatalf("list=%+v", list)
	}
}

// Un token de stage cuyo stage desapareció (borrado con foreign_keys=OFF) no
// autentica: no puede volverse un token de todo el proyecto.
func TestAuthenticateTokenWithOrphanedStage(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")
	auditfy := f.project(t, "auditfy")
	staging := f.stage(t, auditfy, "staging")
	tok, err := f.createToken.Execute(ctx, newToken("ci", "auditfy", "staging", u.ID, 0))
	if err != nil {
		t.Fatal(err)
	}
	f.rawExec(t, "DELETE FROM stages WHERE id = ?", staging)
	if p, err := f.authToken.Execute(ctx, tok.Plain); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("esperaba ErrUnauthenticated, got p=%+v err=%v", p, err)
	}
}
