package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/secrets/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/secrets/seedtest"
	"github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

// El repositorio no cifra: estos "cifrados" son bytes cualquiera.
func row(key, value string, secret bool, at time.Time) entity.EncryptedEnvVar {
	return entity.EncryptedEnvVar{Key: key, Ciphertext: []byte(value), Secret: secret, UpdatedAt: at}
}

func keys(vars []entity.EncryptedEnvVar) []string {
	out := []string{}
	for _, v := range vars {
		out = append(out, v.Key)
	}
	return out
}

func TestReplaceAndPatchEnv(t *testing.T) {
	db := sqlitetest.New(t)
	repo := NewEnvRepository(db)
	ctx := context.Background()
	_, appID := seedtest.Auditfy(t, db)
	o := entity.AppOwner(appID)
	t1 := seedtest.T0.Add(time.Minute)

	err := repo.Replace(ctx, o, []entity.EncryptedEnvVar{row("B", "cb", true, t1), row("A", "ca", false, t1)})
	if err != nil {
		t.Fatal(err)
	}
	vars, err := repo.List(ctx, o)
	if err != nil || len(vars) != 2 || vars[0].Key != "A" || vars[0].Secret || !vars[1].Secret ||
		string(vars[1].Ciphertext) != "cb" || !vars[1].UpdatedAt.Equal(t1) {
		t.Fatalf("vars=%+v err=%v", vars, err)
	}

	// Replace borra lo que no viene.
	if err := repo.Replace(ctx, o, []entity.EncryptedEnvVar{row("C", "cc", true, t1)}); err != nil {
		t.Fatal(err)
	}
	vars, _ = repo.List(ctx, o)
	if got := keys(vars); len(got) != 1 || got[0] != "C" {
		t.Fatalf("keys=%v", got)
	}

	// Patch: agrega D, reemplaza C, y borra una clave inexistente sin fallar.
	t2 := t1.Add(time.Minute)
	if err := repo.Patch(ctx, o, []entity.EncryptedEnvVar{row("C", "c2", false, t2), row("D", "cd", true, t2)}, []string{"Z"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Patch(ctx, o, nil, []string{"D"}); err != nil {
		t.Fatal(err)
	}
	vars, _ = repo.List(ctx, o)
	if len(vars) != 1 || vars[0].Key != "C" || string(vars[0].Ciphertext) != "c2" || vars[0].Secret || !vars[0].UpdatedAt.Equal(t2) {
		t.Fatalf("vars=%+v", vars)
	}

	// Sin variables, List da un slice vacío, no nil.
	if err := repo.Replace(ctx, o, nil); err != nil {
		t.Fatal(err)
	}
	if vars, err := repo.List(ctx, o); err != nil || vars == nil || len(vars) != 0 {
		t.Fatalf("vars=%#v err=%v", vars, err)
	}
}

func TestEnvIsolatedPerOwnerAndCascades(t *testing.T) {
	db := sqlitetest.New(t)
	repo := NewEnvRepository(db)
	ctx := context.Background()
	stageID, appID := seedtest.Auditfy(t, db)
	// Un stage y una app con el mismo id no comparten variables.
	otherStage := seedtest.Stage(t, db, seedtest.Project(t, db, "otro"), "staging")

	repo.Replace(ctx, entity.StageOwner(stageID), []entity.EncryptedEnvVar{row("A", "stage", true, seedtest.T0)})
	repo.Replace(ctx, entity.AppOwner(appID), []entity.EncryptedEnvVar{row("A", "app", true, seedtest.T0)})
	repo.Replace(ctx, entity.StageOwner(otherStage), []entity.EncryptedEnvVar{row("A", "otro", true, seedtest.T0)})

	sv, _ := repo.List(ctx, entity.StageOwner(stageID))
	av, _ := repo.List(ctx, entity.AppOwner(appID))
	ov, _ := repo.List(ctx, entity.StageOwner(otherStage))
	if len(sv) != 1 || string(sv[0].Ciphertext) != "stage" || len(av) != 1 || string(av[0].Ciphertext) != "app" ||
		len(ov) != 1 || string(ov[0].Ciphertext) != "otro" {
		t.Fatalf("stage=%+v app=%+v otro=%+v", sv, av, ov)
	}

	// Borrar la app (lo hace catalog) borra sus variables en cascada.
	if _, err := db.Conn(ctx).ExecContext(ctx, `DELETE FROM apps WHERE id = ?`, appID); err != nil {
		t.Fatal(err)
	}
	if n := seedtest.Count(t, db, `SELECT COUNT(*) FROM env_vars WHERE app_id IS NOT NULL`); n != 0 {
		t.Fatalf("borrar la app debe borrar sus variables, quedan %d", n)
	}
	// Y borrar el stage (ya sin apps) borra las suyas.
	if _, err := db.Conn(ctx).ExecContext(ctx, `DELETE FROM stages WHERE id = ?`, stageID); err != nil {
		t.Fatal(err)
	}
	if n := seedtest.Count(t, db, `SELECT COUNT(*) FROM env_vars WHERE stage_id = ?`, stageID); n != 0 {
		t.Fatalf("borrar el stage debe borrar sus variables, quedan %d", n)
	}
}

func TestEnvUnknownOwner(t *testing.T) {
	db := sqlitetest.New(t)
	repo := NewEnvRepository(db)
	ctx := context.Background()
	vars := []entity.EncryptedEnvVar{row("A", "x", true, seedtest.T0)}

	if err := repo.Replace(ctx, entity.AppOwner(999), vars); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("app inexistente: got %v", err)
	}
	if err := repo.Patch(ctx, entity.StageOwner(999), vars, nil); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("stage inexistente: got %v", err)
	}

	// Un dueño sin resolver nunca llega a la base.
	if _, err := repo.List(ctx, entity.Owner{}); err == nil {
		t.Error("List con un dueño vacío debería fallar")
	}
	if err := repo.Replace(ctx, entity.Owner{Kind: "project", ID: 1}, vars); err == nil {
		t.Error("Replace con un tipo de dueño desconocido debería fallar")
	}
	if err := repo.Patch(ctx, entity.Owner{}, nil, []string{"A"}); err == nil {
		t.Error("Patch con un dueño vacío debería fallar")
	}
}

// Replace y Patch participan en el InTx del caso de uso: si este hace
// rollback, el cambio tampoco queda.
func TestEnvJoinsTransaction(t *testing.T) {
	db := sqlitetest.New(t)
	repo := NewEnvRepository(db)
	ctx := context.Background()
	_, appID := seedtest.Auditfy(t, db)
	o := entity.AppOwner(appID)
	repo.Replace(ctx, o, []entity.EncryptedEnvVar{row("A", "x", true, seedtest.T0)})

	boom := errors.New("boom")
	err := db.InTx(ctx, func(ctx context.Context) error {
		if err := repo.Replace(ctx, o, []entity.EncryptedEnvVar{row("B", "y", true, seedtest.T0)}); err != nil {
			return err
		}
		if err := repo.Patch(ctx, o, []entity.EncryptedEnvVar{row("C", "z", true, seedtest.T0)}, []string{"B"}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
	vars, _ := repo.List(ctx, o)
	if got := keys(vars); len(got) != 1 || got[0] != "A" {
		t.Fatalf("el rollback debía dejar solo A: %v", got)
	}
}
