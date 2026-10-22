// Package mcp registra las herramientas MCP de deploy (contrato §8.3):
// get_app_status, get_app_logs, list_deploys, get_deploy y read_deploy_log
// (lectura), y deploy y rollback (escritura). Usan los mismos casos de uso
// que REST; el alcance lo revisa el tools.Kit antes de llamarlos.
package mcp

import (
	"context"
	"time"
	"unicode/utf8"

	"github.com/CPU-commits/muelle/src/cmd/mcp/tools"
	"github.com/CPU-commits/muelle/src/internal/deploy/app/dto"
	"github.com/CPU-commits/muelle/src/internal/deploy/app/usecases"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// UseCases son los casos de uso que usan las herramientas.
type UseCases struct {
	// CheckApp es el not_found de una app que no existe (CreateDeploy.CheckApp).
	CheckApp     func(ctx context.Context, ref dto.AppRef) error
	GetStatus    *usecases.GetStatus
	AppLogs      *usecases.AppLogs
	ListDeploys  *usecases.ListDeploys
	GetDeploy    *usecases.GetDeploy
	TailLog      *usecases.TailLog
	ReadLog      *usecases.ReadLog
	FollowLog    *usecases.FollowLog
	CreateDeploy *usecases.CreateDeploy
	Rollback     *usecases.Rollback
}

// Config son los límites de las herramientas; los tests los acortan.
type Config struct {
	// Slots son los 8 streams por principal que comparten con los SSE.
	Slots *usecases.StreamSlots
	// LogReadTimeout es cuánto lee get_app_logs (15 s): más corto que
	// LogCallTimeout, así la llamada vuelve con incomplete y no con internal.
	LogReadTimeout time.Duration
	// LogCallTimeout es el plazo de get_app_logs (el de toda llamada, 20 s).
	LogCallTimeout time.Duration
	// WaitPoll es cada cuánto get_deploy con wait_s vuelve a leer el deploy
	// además de los avisos del Hub (la red de seguridad, como el heartbeat
	// del SSE).
	WaitPoll time.Duration
	Clock    clock.Clock
}

// DefaultConfig son los límites del contrato §8.5.
func DefaultConfig(slots *usecases.StreamSlots, c clock.Clock) Config {
	return Config{Slots: slots, LogReadTimeout: 15 * time.Second, LogCallTimeout: tools.DefaultTimeout,
		WaitPoll: time.Second, Clock: c}
}

// MaxErrorChars es lo que mide el error de una tarea o de un deploy en
// get_app_status y list_deploys.
const MaxErrorChars = 300

type appIn struct {
	Project string `json:"project" jsonschema:"el slug del proyecto"`
	Stage   string `json:"stage" jsonschema:"el slug del stage"`
	App     string `json:"app" jsonschema:"el slug de la app"`
}

func (in appIn) ref() dto.AppRef {
	return dto.AppRef{Project: in.Project, Stage: in.Stage, App: in.App}
}
func (in appIn) scope() *tools.Scope    { return &tools.Scope{Project: in.Project, Stage: in.Stage} }
func millis(t time.Time) time.Time      { return t.UTC().Truncate(time.Millisecond) }
func millisPtr(t *time.Time) *time.Time { return mapPtr(t, millis) }

func mapPtr[T any](p *T, f func(T) T) *T {
	if p == nil {
		return nil
	}
	v := f(*p)
	return &v
}

// cutChars corta s a max caracteres (runas), con tools.Ellipsis al final.
func cutChars(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	n := 0
	for i := range s {
		if n == max-1 {
			return s[:i] + tools.Ellipsis
		}
		n++
	}
	return s
}

// Register registra las siete herramientas de deploy.
func Register(k *tools.Kit, uc UseCases, cfg Config) {
	registerStatus(k, uc)
	registerLogs(k, uc, cfg)
	registerDeploys(k, uc, cfg)
	registerWrites(k, uc)
}

// principal es el Principal del ctx de una llamada (el kit siempre lo pone).
func principal(ctx context.Context) entity.Principal {
	p, _ := entity.PrincipalFrom(ctx)
	return p
}
