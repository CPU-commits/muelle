package service

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/CPU-commits/muelle/src/shared/domain/naming"
)

// updateSkew es el margen para decidir si un UpdateStatus es de este deploy:
// uno que empezó hasta 2 s antes de aplicar cuenta (los relojes de Swarm y de
// muelle pueden no coincidir al milisegundo, §7 paso 6).
const updateSkew = 2 * time.Second

// Verdict es el resultado de mirar los servicios una vez (§7 paso 6).
type Verdict struct {
	Done   bool // todos convergieron
	Failed bool // alguno falló: Service, ServiceID y Reason dicen cuál y por qué
	// Service es el nombre corto del que falló; ServiceID, su id en Swarm
	// (para buscar su última tarea fallida y sus logs).
	Service   string
	ServiceID string
	// Reason es por qué falló, con el Message de Swarm: el deploy cierra con
	// "<Service>: <Reason>".
	Reason   string
	Progress map[string]string // nombre corto → "1/2 réplicas corriendo"
}

// Evaluate aplica la regla del paso 6 a los servicios esperados (nombres de
// Swarm) con lo que devolvió Swarm. appliedAt es la hora de justo antes de
// ejecutar docker stack deploy: un UpdateStatus de este deploy es uno con
// StartedAt ≥ appliedAt−2s. settleFrom es cuando docker stack deploy volvió
// (decisión 6 del plan): un servicio que ya existía y no tiene UpdateStatus de
// este deploy converge cuando pasó settle desde settleFrom (no desde
// appliedAt: el CLI puede tardar más que settle) y corren las deseadas.
//
// Para cada servicio:
//   - con un UpdateStatus de este deploy: completed converge; rollback_started,
//     rollback_completed, rollback_paused y paused fallan; updating (o un
//     estado desconocido) espera;
//   - creado por este deploy (CreatedAt ≥ appliedAt−2s): converge cuando
//     corren las deseadas (Replicas, o DesiredTasks en uno global). Con
//     healthcheck, Swarm cuenta como corriendo solo las sanas;
//   - si no, ya existía y su plantilla no cambió: converge pasado el margen y
//     con las deseadas corriendo;
//   - si todavía no está en Swarm, espera.
//
// Si varios fallan, Verdict nombra el primero por nombre.
func Evaluate(expected []string, services []SwarmService, appliedAt, settleFrom, now time.Time, settle time.Duration) Verdict {
	byName := make(map[string]SwarmService, len(services))
	for _, s := range services {
		byName[s.Name] = s
	}
	threshold := appliedAt.Add(-updateSkew)
	settled := !now.Before(settleFrom.Add(settle))
	v := Verdict{Done: true, Progress: make(map[string]string, len(expected))}
	for _, name := range slices.Sorted(slices.Values(expected)) {
		short := shortName(name)
		s, ok := byName[name]
		if !ok {
			v.Progress[short] = "todavía no está en Swarm"
			v.Done = false
			continue
		}
		desired := s.DesiredTasks
		if s.Replicas != nil {
			desired = *s.Replicas
		}
		v.Progress[short] = fmt.Sprintf("%d/%d réplicas corriendo", s.RunningTasks, desired)
		running := s.RunningTasks >= desired

		var converged bool
		switch u := s.Update; {
		case u != nil && u.StartedAt != nil && !u.StartedAt.Before(threshold):
			switch u.State {
			case UpdateCompleted:
				converged = true
			case UpdateRollbackStarted, UpdateRollbackCompleted, UpdateRollbackPaused, UpdatePaused:
				if !v.Failed {
					v.Failed, v.Service, v.ServiceID, v.Reason = true, short, s.ID, failReason(u)
				}
			}
		case !s.CreatedAt.Before(threshold):
			converged = running
		default:
			converged = settled && running
		}
		if !converged {
			v.Done = false
		}
	}
	if v.Failed {
		v.Done = false
	}
	return v
}

// failReason es el motivo de una actualización que Swarm deshizo o pausó.
func failReason(u *UpdateStatus) string {
	var r string
	switch u.State {
	case UpdatePaused:
		r = "Swarm pausó la actualización (paused)"
	case UpdateRollbackPaused:
		r = "Swarm hizo rollback y lo pausó (rollback_paused)"
	default:
		r = "Swarm hizo rollback (" + u.State + ")"
	}
	if msg := strings.TrimSpace(u.Message); msg != "" {
		r += ": " + msg
	}
	return r
}

// shortName es el nombre corto de un servicio de Swarm: lo que sigue al
// último "_" (proyecto_stage_app_servicio, naming.Service; un slug no lleva "_").
func shortName(swarm string) string {
	if i := strings.LastIndexByte(swarm, '_'); i >= 0 {
		return swarm[i+1:]
	}
	return swarm
}

// Applied dice cuántos de los servicios esperados llegaron a Swarm con un
// deploy (contrato §1.2).
type Applied int

const (
	AppliedNone    Applied = iota // ninguno lleva su muelle.deploy: Swarm no cambió
	AppliedPartial                // algunos: el CLI se cortó a mitad
	AppliedAll                    // todos
)

func (a Applied) String() string {
	switch a {
	case AppliedNone:
		return "none"
	case AppliedPartial:
		return "partial"
	case AppliedAll:
		return "all"
	}
	return "Applied(" + strconv.Itoa(int(a)) + ")"
}

// AppliedState dice cuántos de los servicios esperados (nombres de Swarm)
// llevan muelle.deploy = deployID (contrato §1.2): AppliedNone, AppliedPartial
// o AppliedAll. Sin servicios esperados, AppliedNone.
func AppliedState(expected []string, services []SwarmService, deployID int64) Applied {
	id := strconv.FormatInt(deployID, 10)
	labeled := map[string]bool{}
	for _, s := range services {
		if s.Labels[naming.LabelDeploy] == id {
			labeled[s.Name] = true
		}
	}
	n := 0
	for _, name := range expected {
		if labeled[name] {
			n++
		}
	}
	switch {
	case n == 0:
		return AppliedNone
	case n == len(expected):
		return AppliedAll
	default:
		return AppliedPartial
	}
}
