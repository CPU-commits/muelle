package usecases

import (
	"bytes"
	"context"
	"errors"

	"github.com/CPU-commits/muelle/src/internal/deploy/app/dto"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// FollowLog es el log de un deploy en vivo (contrato §2.6): los mismos bytes
// que el polling (§2.5), y el Hub para esperar lo nuevo.
type FollowLog struct{ d Deps }

func NewFollowLog(d Deps) *FollowLog { return &FollowLog{d: d} }

// Open valida como el polling, en el mismo orden: la app (404), el deploy de
// esa app (404) y el offset (422: no es un entero ≥ 0, que llega como un
// negativo, o pasa el largo del log). Todo antes de abrir el stream.
func (u *FollowLog) Open(ctx context.Context, in dto.ReadLog) (*LogFollower, error) {
	app, err := u.d.Apps.Get(ctx, in.Project, in.Stage, in.App)
	if err != nil {
		return nil, err
	}
	if in.Offset < 0 {
		if _, err := u.d.Deploys.Get(ctx, app.ID, in.ID); err != nil {
			return nil, err
		}
		return nil, ErrBadOffset
	}
	// Una lectura de 0 bytes: el estado, y si el offset pasa el largo.
	c, err := u.d.Deploys.ReadLog(ctx, app.ID, in.ID, in.Offset, 0)
	if errors.Is(err, service.ErrOffsetBeyond) {
		return nil, validate.Errorf("offset", "pasa el largo del log")
	}
	if err != nil {
		return nil, err
	}
	return &LogFollower{d: u.d, appID: app.ID, id: in.ID, offset: in.Offset, status: c.Status}, nil
}

// LogFollower lee el log de un deploy desde un offset, a medida que crece.
// Lo usa una sola goroutine.
type LogFollower struct {
	d      Deps
	appID  int64
	id     int64
	offset int
	status entity.Status
}

// Status es el estado de la última lectura (o el de Open).
func (f *LogFollower) Status() entity.Status { return f.status }

// Subscribe es el canal que se cierra con el próximo cambio del deploy (una
// línea o un estado), y su baja. Se toma antes de Read: así no se pierde lo
// que cambie entre la lectura y la espera.
func (f *LogFollower) Subscribe() (<-chan struct{}, func()) { return f.d.Hub.Subscribe(f.id) }

// Read lee todo lo que hay desde el último offset, en líneas completas (de a
// 64 KiB), y devuelve cada línea con el offset que le sigue, y el estado de
// la última lectura. Como el estado final se guarda después de las últimas
// líneas (o con ellas, en la misma transacción), si es final ya están todas.
// repository.ErrNotFound si el deploy ya no existe (la retención, o se borró
// la app).
func (f *LogFollower) Read(ctx context.Context) ([]dto.LogLine, entity.Status, error) {
	var out []dto.LogLine
	for {
		c, err := f.d.Deploys.ReadLog(ctx, f.appID, f.id, f.offset, LogPageBytes)
		if err != nil {
			return nil, "", err
		}
		f.status = c.Status
		chunk := c.Chunk
		for len(chunk) > 0 {
			i := bytes.IndexByte(chunk, '\n')
			f.offset += i + 1
			out = append(out, dto.LogLine{Text: string(chunk[:i]), Offset: f.offset})
			chunk = chunk[i+1:]
		}
		// Sin nada nuevo, o ya al final: listo. Un pedazo vacío antes del
		// final no debería pasar (las líneas miden a lo más 8 KiB).
		if len(c.Chunk) == 0 || f.offset >= c.Length {
			return out, f.status, nil
		}
	}
}
