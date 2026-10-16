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

// Subscribe es Wait con su baja: un stream que se va no deja su canal en el
// Hub (un deploy que ya terminó no vuelve a recibir un Notify que lo borre).
func TestHubSubscribe(t *testing.T) {
	h := NewHub()
	a, cancelA := h.Subscribe(1)
	b, cancelB := h.Subscribe(1)
	cancelA()
	cancelA() // dos veces no cuenta doble
	if n := h.Pending(); n != 1 {
		t.Fatalf("con b esperando: %d canales", n)
	}
	h.Notify(1)
	if !closed(a) || !closed(b) {
		t.Fatal("Notify no despertó a los suscritos")
	}
	// La baja de un canal ya cerrado no toca el siguiente.
	c, cancelC := h.Subscribe(1)
	cancelB()
	if closed(c) || h.Pending() != 1 {
		t.Fatal("la baja de un canal viejo tocó el nuevo")
	}
	cancelC()
	if n := h.Pending(); n != 0 {
		t.Fatalf("sin nadie esperando quedaron %d canales", n)
	}
	// Wait no se da de baja: queda hasta el próximo Notify.
	w := h.Wait(2)
	_, cancelD := h.Subscribe(2)
	cancelD()
	if h.Pending() != 1 || closed(w) {
		t.Fatal("la baja de un Subscribe borró el canal de un Wait")
	}
}

// FollowLog valida como el polling (contrato §2.6: 404 de la app, 404 del
// deploy y 422 del offset, en ese orden) y después lee todo lo que hay desde
// el offset, línea por línea, con el offset después de cada una.
func TestFollowLog(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	d := &entity.Deploy{AppID: f.apiID, Trigger: entity.TriggerAPI, ActorType: entity.ActorToken, ActorName: "ci", CreatedAt: t0}
	if err := f.deploys.Create(ctx, d); err != nil {
		t.Fatal(err)
	}
	other := &entity.Deploy{AppID: f.infraID, Trigger: entity.TriggerAPI, ActorType: entity.ActorToken, ActorName: "ci", CreatedAt: t0}
	if err := f.deploys.Create(ctx, other); err != nil {
		t.Fatal(err)
	}
	u := NewFollowLog(Deps{Tx: f.db, Apps: f.apps, Deploys: f.deploys, Queue: f.queue, Hub: f.hub, Clock: f.clock, Audit: f.audit})
	ref := func(app string, id int64, offset int) dto.ReadLog {
		return dto.ReadLog{DeployRef: dto.DeployRef{AppRef: dto.AppRef{Project: "auditfy", Stage: "staging", App: app}, ID: id}, Offset: offset}
	}

	for _, c := range []struct {
		name string
		in   dto.ReadLog
		want string // "" es ErrNotFound
	}{
		{"app que no existe", ref("nada", d.ID, -1), ""},
		{"deploy de otra app", ref("api", other.ID, -1), ""},
		{"offset inválido", ref("api", d.ID, -1), "offset: tiene que ser un entero ≥ 0"},
		{"offset más allá del log", ref("api", d.ID, 1), "offset: pasa el largo del log"},
	} {
		_, err := u.Open(ctx, c.in)
		if c.want == "" {
			if !errors.Is(err, srepo.ErrNotFound) {
				t.Errorf("%s: %v", c.name, err)
			}
			continue
		}
		if err == nil || validationMessage(t, err) != c.want {
			t.Errorf("%s: %v", c.name, err)
		}
	}

	log := NewLogWriter(f.deploys, f.hub, f.clock).For(d.ID, service.Masker{})
	for _, l := range []string{"uno", "dos", strings.Repeat("x", 7000)} {
		if err := log.Line("%s", l); err != nil {
			t.Fatal(err)
		}
	}
	fl, err := u.Open(ctx, ref("api", d.ID, 0))
	if err != nil {
		t.Fatal(err)
	}
	if fl.Status() != entity.StatusQueued {
		t.Fatalf("estado al abrir: %s", fl.Status())
	}
	lines, status, err := fl.Read(ctx)
	if err != nil || status != entity.StatusQueued || len(lines) != 3 {
		t.Fatalf("Read: %d líneas, %s, %v", len(lines), status, err)
	}
	offset := 0
	for i, l := range lines {
		offset += len(l.Text) + 1
		if l.Offset != offset || strings.Contains(l.Text, "\n") {
			t.Fatalf("línea %d: offset %d, esperaba %d (%q)", i, l.Offset, offset, l.Text)
		}
	}
	if !strings.HasSuffix(lines[1].Text, " dos") {
		t.Fatalf("segunda línea: %q", lines[1].Text)
	}
	// Ya no hay nada nuevo; desde un offset del medio, lo que sigue.
	if again, _, err := fl.Read(ctx); err != nil || len(again) != 0 {
		t.Fatalf("sin nada nuevo: %v %v", again, err)
	}
	fl2, err := u.Open(ctx, ref("api", d.ID, lines[0].Offset))
	if err != nil {
		t.Fatal(err)
	}
	if rest, _, _ := fl2.Read(ctx); len(rest) != 2 || rest[1].Offset != offset {
		t.Fatalf("desde la primera línea: %+v", rest)
	}

	// Un log de más de 64 KiB (una lectura del repositorio) llega entero.
	for range 20 {
		if err := log.Line("%s", strings.Repeat("y", 7000)); err != nil {
			t.Fatal(err)
		}
	}
	more, _, err := fl.Read(ctx)
	if err != nil || len(more) != 20 || more[19].Offset <= LogPageBytes {
		t.Fatalf("más de 64 KiB: %d líneas, %v", len(more), err)
	}

	// Suscribirse antes de leer: lo que cambie después despierta al stream.
	wait, cancel := fl.Subscribe()
	defer cancel()
	if err := log.Line("cuatro"); err != nil {
		t.Fatal(err)
	}
	if !closed(wait) {
		t.Fatal("la línea nueva no despertó al stream")
	}
}
