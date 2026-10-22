package usecases

import (
	"bytes"
	"context"

	"github.com/CPU-commits/muelle/src/internal/deploy/app/dto"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
)

// TailLog es el final del log de un deploy en líneas completas (get_deploy
// por MCP, contrato §8.3 #9): con el largo L del log, lee desde
// max(0, L − Bytes) y avanza hasta el primer comienzo de línea. Si la última
// línea sola mide más que Bytes, la devuelve igual, entera: quien la muestra
// la corta. Lee a lo más Bytes del log (o una línea, de hasta 8 KiB).
type TailLog struct{ d Deps }

func NewTailLog(d Deps) *TailLog { return &TailLog{d: d} }

// Execute responde los mismos 404 que el polling: la app, y el deploy de esa
// app.
func (u *TailLog) Execute(ctx context.Context, in dto.TailLog) (dto.LogTail, error) {
	app, err := u.d.Apps.Get(ctx, in.Project, in.Stage, in.App)
	if err != nil {
		return dto.LogTail{}, err
	}
	head, err := u.d.Deploys.ReadLog(ctx, app.ID, in.ID, 0, 0)
	if err != nil {
		return dto.LogTail{}, err
	}
	end := head.Length
	out := dto.LogTail{FromOffset: end, Length: end, Status: head.Status, SupersededBy: head.SupersededBy}
	if end == 0 || in.Bytes <= 0 {
		return out, nil
	}
	start := max(0, end-in.Bytes)
	// Un byte antes de start dice si start ya es un comienzo de línea.
	from := max(0, start-1)
	c, err := u.d.Deploys.ReadLog(ctx, app.ID, in.ID, from, end-from)
	if err != nil {
		return dto.LogTail{}, err
	}
	skip := 0
	if from < start {
		if i := bytes.IndexByte(c.Chunk, '\n'); i >= 0 {
			skip = i + 1
		} else {
			skip = len(c.Chunk)
		}
	}
	if skip == len(c.Chunk) {
		// La última línea sola no entra en Bytes: se lee entera (mide a lo
		// más MaxLineBytes y su \n), con el \n de la anterior adelante.
		from = max(0, end-service.MaxLineBytes-2)
		if c, err = u.d.Deploys.ReadLog(ctx, app.ID, in.ID, from, end-from); err != nil {
			return dto.LogTail{}, err
		}
		skip = 0
		if body := c.Chunk[:max(0, len(c.Chunk)-1)]; from > 0 || bytes.IndexByte(body, '\n') >= 0 {
			skip = bytes.LastIndexByte(body, '\n') + 1
		}
	}
	out.FromOffset, out.Length, out.Status, out.SupersededBy = from+skip, c.Length, c.Status, c.SupersededBy
	offset := out.FromOffset
	for chunk := c.Chunk[skip:]; len(chunk) > 0; {
		i := bytes.IndexByte(chunk, '\n')
		offset += i + 1
		out.Lines = append(out.Lines, dto.LogLine{Text: string(chunk[:i]), Offset: offset})
		chunk = chunk[i+1:]
	}
	return out, nil
}
