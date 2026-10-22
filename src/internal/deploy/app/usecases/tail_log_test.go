package usecases

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/CPU-commits/muelle/src/internal/deploy/app/dto"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	srepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// TailLog es el final del log de un deploy en líneas completas (get_deploy
// por MCP, contrato §8.3 #9): lee a lo más Bytes del final, desde el primer
// comienzo de línea; si la última línea sola no entra, la devuelve igual
// (el adaptador la corta).
func TestTailLog(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	d := &entity.Deploy{AppID: f.apiID, Trigger: entity.TriggerAPI, ActorType: entity.ActorToken, ActorName: "ci", CreatedAt: t0}
	if err := f.deploys.Create(ctx, d); err != nil {
		t.Fatal(err)
	}
	u := NewTailLog(f.deps())
	ref := func(app string, id int64, n int) dto.TailLog {
		return dto.TailLog{DeployRef: dto.DeployRef{AppRef: dto.AppRef{Project: "auditfy", Stage: "staging", App: app}, ID: id}, Bytes: n}
	}
	if _, err := u.Execute(ctx, ref("nada", d.ID, 100)); !errors.Is(err, srepo.ErrNotFound) {
		t.Fatalf("app que no existe: %v", err)
	}
	if _, err := u.Execute(ctx, ref("infra", d.ID, 100)); !errors.Is(err, srepo.ErrNotFound) {
		t.Fatalf("deploy de otra app: %v", err)
	}

	// Un log vacío.
	tail, err := u.Execute(ctx, ref("api", d.ID, 100))
	if err != nil || len(tail.Lines) != 0 || tail.FromOffset != 0 || tail.Length != 0 || tail.Status != entity.StatusQueued {
		t.Fatalf("vacío: %+v %v", tail, err)
	}

	log := NewLogWriter(f.deploys, f.hub, f.clock).For(d.ID, service.Masker{})
	for _, l := range []string{"uno", "dos", "tres"} {
		if err := log.Line("%s", l); err != nil {
			t.Fatal(err)
		}
	}
	// Más corto que lo pedido: todo, desde 0.
	tail, err = u.Execute(ctx, ref("api", d.ID, 4096))
	if err != nil || len(tail.Lines) != 3 || tail.FromOffset != 0 || !strings.HasSuffix(tail.Lines[2].Text, " tres") ||
		tail.Lines[2].Offset != tail.Length {
		t.Fatalf("corto: %+v %v", tail, err)
	}
	line := len(tail.Lines[0].Text) + 1 // "<hora> uno\n": las tres miden lo mismo salvo el texto
	// Más largo: las últimas líneas completas que entran en Bytes.
	n := tail.Length - line + 2 // no alcanza para la primera entera
	tail, err = u.Execute(ctx, ref("api", d.ID, n))
	if err != nil || len(tail.Lines) != 2 || tail.FromOffset != line || !strings.HasSuffix(tail.Lines[0].Text, " dos") {
		t.Fatalf("largo: %+v %v", tail, err)
	}
	// Justo en un comienzo de línea.
	tail, err = u.Execute(ctx, ref("api", d.ID, tail.Length-line))
	if err != nil || len(tail.Lines) != 2 || tail.FromOffset != line {
		t.Fatalf("en un comienzo de línea: %+v %v", tail, err)
	}

	// La última línea sola no entra: sale igual, entera.
	if err := log.Line("%s", strings.Repeat("x", 7000)); err != nil {
		t.Fatal(err)
	}
	tail, err = u.Execute(ctx, ref("api", d.ID, 100))
	if err != nil || len(tail.Lines) != 1 || !strings.HasSuffix(tail.Lines[0].Text, strings.Repeat("x", 7000)) ||
		tail.Lines[0].Offset != tail.Length || tail.FromOffset != tail.Length-len(tail.Lines[0].Text)-1 {
		t.Fatalf("una última línea que no entra: %d líneas, from %d, length %d, %v", len(tail.Lines), tail.FromOffset, tail.Length, err)
	}
	// Una línea de 8 KiB (el máximo) en un log más largo también.
	for range 10 {
		if err := log.Line("%s", strings.Repeat("y", 9000)); err != nil {
			t.Fatal(err)
		}
	}
	tail, err = u.Execute(ctx, ref("api", d.ID, 10))
	if err != nil || len(tail.Lines) != 1 || len(tail.Lines[0].Text) != service.MaxLineBytes {
		t.Fatalf("una línea de 8 KiB: %d líneas (%d bytes), %v", len(tail.Lines), len(tail.Lines[0].Text), err)
	}
}
