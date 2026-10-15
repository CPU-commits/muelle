package service

import (
	"context"
	"errors"
	"testing"

	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

// MarkOnce guarda la marca solo la primera vez, y dentro de la transacción del ctx.
func TestDeploySettingsMarkOnce(t *testing.T) {
	db := sqlitetest.New(t)
	s := NewDeploySettings(db)
	ctx := context.Background()

	boom := errors.New("boom")
	err := db.InTx(ctx, func(ctx context.Context) error {
		if ok, err := s.MarkOnce(ctx, "init_registry_applied"); err != nil || !ok {
			t.Fatalf("dentro de la transacción: %v, %v", ok, err)
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	// El rollback se llevó la marca: la primera vez sigue siendo la primera.
	if ok, err := s.MarkOnce(ctx, "init_registry_applied"); err != nil || !ok {
		t.Fatalf("primera: %v, %v", ok, err)
	}
	if ok, err := s.MarkOnce(ctx, "init_registry_applied"); err != nil || ok {
		t.Fatalf("segunda: %v, %v", ok, err)
	}
	var value string
	var encrypted bool
	if err := db.Conn(ctx).QueryRowContext(ctx, `SELECT value, encrypted FROM settings WHERE key = 'init_registry_applied'`).Scan(&value, &encrypted); err != nil {
		t.Fatal(err)
	}
	if value != "true" || encrypted {
		t.Fatalf("value=%q encrypted=%v", value, encrypted)
	}
}

// manager_node_id: Get da "" mientras no está; SetIfMissing lo guarda sin
// cifrar y nunca pisa el que ya estaba (devuelve el que quedó).
func TestDeploySettingsManagerNode(t *testing.T) {
	db := sqlitetest.New(t)
	s := NewDeploySettings(db)
	ctx := context.Background()
	if id, err := s.Get(ctx); err != nil || id != "" {
		t.Fatalf("sin guardar: %q, %v", id, err)
	}
	if id, err := s.SetIfMissing(ctx, "node-1"); err != nil || id != "node-1" {
		t.Fatalf("primera: %q, %v", id, err)
	}
	if id, err := s.SetIfMissing(ctx, "node-2"); err != nil || id != "node-1" {
		t.Fatalf("no pisa: %q, %v", id, err)
	}
	if id, err := s.Get(ctx); err != nil || id != "node-1" {
		t.Fatalf("Get: %q, %v", id, err)
	}
	var value string
	var encrypted bool
	if err := db.Conn(ctx).QueryRowContext(ctx, `SELECT value, encrypted FROM settings WHERE key = 'manager_node_id'`).Scan(&value, &encrypted); err != nil {
		t.Fatal(err)
	}
	if value != "node-1" || encrypted {
		t.Fatalf("value=%q encrypted=%v", value, encrypted)
	}
}
