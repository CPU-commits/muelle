package usecases

import (
	"context"
	"fmt"

	"github.com/CPU-commits/muelle/src/internal/deploy/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	"github.com/CPU-commits/muelle/src/package/clock"
)

// LogWriter escribe el log de un deploy (contrato §2.5): formatea y enmascara
// cada línea (service.FormatLine), la guarda (AppendLog, con el tope de 256
// KB) y recién después avisa a los streams (Hub.Notify): lo que se ve por SSE
// ya está en ?offset.
//
// Avisa al escribir, así que se usa solo fuera de una transacción: dentro de
// un InTx avisaría de algo que todavía no está confirmado. Las líneas de
// cierre que van en la misma transacción que el estado final se escriben con
// AppendLog directamente, y se avisa después del commit.
type LogWriter struct {
	deploys repository.DeployRepository
	hub     *Hub
	clock   clock.Clock
}

func NewLogWriter(deploys repository.DeployRepository, hub *Hub, c clock.Clock) *LogWriter {
	return &LogWriter{deploys: deploys, hub: hub, clock: c}
}

// For es el log del deploy id, enmascarado con m.
func (w *LogWriter) For(id int64, m service.Masker) *DeployLog {
	return &DeployLog{w: w, id: id, m: m}
}

// DeployLog es el log de un deploy, con su Masker.
type DeployLog struct {
	w  *LogWriter
	id int64
	m  service.Masker
}

// Format formatea textos como líneas del log (con la hora del reloj), y la
// marca de log truncado, para escribirlas con AppendLog dentro de una
// transacción.
func (l *DeployLog) Format(texts ...string) (lines []string, marker string) {
	now := l.w.clock.Now()
	lines = make([]string, len(texts))
	for i, t := range texts {
		lines[i] = service.FormatLine(l.m, now, t)
	}
	return lines, service.FormatLine(l.m, now, service.TruncatedMarker)
}

// Line escribe una línea normal (fmt.Sprintf de format y args).
func (l *DeployLog) Line(format string, args ...any) error {
	return l.write(false, fmt.Sprintf(format, args...))
}

// Lines escribe varias líneas normales en una sola escritura (la salida del
// CLI, las últimas líneas de una tarea), tal cual.
func (l *DeployLog) Lines(texts []string) error {
	if len(texts) == 0 {
		return nil
	}
	return l.write(false, texts...)
}

// Final escribe una línea de cierre: entra en la reserva del tope.
func (l *DeployLog) Final(format string, args ...any) error {
	return l.write(true, fmt.Sprintf(format, args...))
}

func (l *DeployLog) write(final bool, texts ...string) error {
	lines, marker := l.Format(texts...)
	// Sin cancelación: una línea que ya se decidió escribir se escribe aunque
	// el ctx del worker se esté cancelando.
	if err := l.w.deploys.AppendLog(context.Background(), l.id, lines, final, marker); err != nil {
		return err
	}
	l.w.hub.Notify(l.id)
	return nil
}
