package usecases

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
)

func closed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// Wait antes de Notify se despierta; uno tomado después espera el siguiente.
// Cada deploy tiene su canal.
func TestHubNotify(t *testing.T) {
	h := NewHub()
	h.Notify(7) // sin nadie esperando, no pasa nada

	w1, w1b, other := h.Wait(1), h.Wait(1), h.Wait(2)
	if closed(w1) || closed(w1b) {
		t.Fatal("cerrado antes del Notify")
	}
	h.Notify(1)
	if !closed(w1) || !closed(w1b) {
		t.Fatal("Notify no despertó a los que esperaban")
	}
	if closed(other) {
		t.Fatal("Notify de 1 despertó a 2")
	}
	w2 := h.Wait(1)
	if closed(w2) {
		t.Fatal("un Wait después del Notify debe esperar el siguiente")
	}
	h.Notify(1)
	select {
	case <-w2:
	case <-time.After(time.Second):
		t.Fatal("el segundo Notify no llegó")
	}
}

// checkingRepo verifica, al guardar, que el Hub todavía no avisó.
type checkingRepo struct {
	repository.DeployRepository
	t    *testing.T
	wait <-chan struct{}
}

func (r checkingRepo) AppendLog(ctx context.Context, id int64, lines []string, final bool, marker string) error {
	if closed(r.wait) {
		r.t.Error("Notify antes de guardar la línea")
	}
	if !strings.HasSuffix(marker, "Z "+service.TruncatedMarker+"\n") {
		r.t.Errorf("la marca no es TruncatedMarker formateada: %q", marker)
	}
	return r.DeployRepository.AppendLog(ctx, id, lines, final, marker)
}

// El log se formatea y enmascara antes de guardarlo, y recién después se
// avisa a los streams: lo que se ve por SSE ya está en ?offset (contrato §2.5).
func TestLogWriterMasksAndNotifies(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	d := entity.Deploy{AppID: f.apiID, Trigger: entity.TriggerPanel, ActorType: entity.ActorUser, ActorName: "ana", CreatedAt: t0}
	if err := f.deploys.Create(ctx, &d); err != nil {
		t.Fatal(err)
	}
	const secret = "s3cr3t-de-la-app"
	m := service.NewMasker([]string{secret})
	// Cada escritura con un Wait nuevo: el repositorio verifica que todavía no
	// se avisó al guardar, y después de escribir el canal tiene que estar cerrado.
	for i, write := range []func(l *DeployLog) error{
		func(l *DeployLog) error { return l.Line("conectando con %s", "postgres://u:"+secret+"@db") },
		func(l *DeployLog) error { return l.Lines([]string{"salida 1 " + secret, "salida 2\r"}) },
		func(l *DeployLog) error { return l.Final("failed: %s", "falló con "+secret) },
	} {
		if i == 1 {
			f.clock.Advance(1500 * time.Millisecond)
		}
		wait := f.hub.Wait(d.ID)
		l := NewLogWriter(checkingRepo{f.deploys, t, wait}, f.hub, f.clock).For(d.ID, m)
		if err := write(l); err != nil {
			t.Fatal(err)
		}
		if !closed(wait) {
			t.Fatalf("escritura %d: no avisó después de guardar", i)
		}
	}

	c, err := f.deploys.ReadLog(ctx, f.apiID, d.ID, 0, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	log := string(c.Chunk)
	if strings.Contains(log, secret) {
		t.Fatalf("el secreto llegó a la base: %s", log)
	}
	want := "2026-10-10T14:00:00.000Z conectando con postgres://u:***@db\n" +
		"2026-10-10T14:00:01.500Z salida 1 ***\n" +
		"2026-10-10T14:00:01.500Z salida 2\n" +
		"2026-10-10T14:00:01.500Z failed: falló con ***\n"
	if log != want {
		t.Fatalf("log:\n%s\nesperaba:\n%s", log, want)
	}
}
