import { getLocale } from '@/shared/ui/i18n'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import type { StatusTone } from '@/shared/ui'
import type { App as CatalogApp } from '@/modules/catalog'
import type { AppHealth, Deploy, DeployStatus, DeployTrigger } from '../../../domain'

// Cómo se nombra y se colorea cada cosa del módulo en la vista. Las pantallas y los
// features usan esto, nunca un switch propio.

export function healthLabel(health: AppHealth): string {
	switch (health) {
		case 'running':
			return m.deploy_health_running()
		case 'degraded':
			return m.deploy_health_degraded()
		case 'down':
			return m.deploy_health_down()
		case 'stopped':
			return m.deploy_health_stopped()
		case 'not_deployed':
			return m.deploy_health_not_deployed()
	}
}

export function healthTone(health: AppHealth): StatusTone {
	switch (health) {
		case 'running':
			return 'ok'
		case 'degraded':
			return 'warn'
		case 'down':
			return 'bad'
		case 'stopped':
		case 'not_deployed':
			return 'idle'
	}
}

export function statusLabel(status: DeployStatus): string {
	switch (status) {
		case 'queued':
			return m.deploy_status_queued()
		case 'running':
			return m.deploy_status_running()
		case 'succeeded':
			return m.deploy_status_succeeded()
		case 'failed':
			return m.deploy_status_failed()
		case 'superseded':
			return m.deploy_status_superseded()
	}
}

/** El punto de un estado. `running` no tiene punto: lleva un spinner (ver DeployStatusLabel). */
export function statusTone(status: DeployStatus): StatusTone {
	switch (status) {
		case 'succeeded':
			return 'ok'
		case 'failed':
			return 'bad'
		case 'running':
			return 'warn'
		case 'queued':
		case 'superseded':
			return 'idle'
	}
}

export function triggerLabel(trigger: DeployTrigger): string {
	switch (trigger) {
		case 'panel':
			return m.deploy_trigger_panel()
		case 'api':
			return m.deploy_trigger_api()
		case 'rollback':
			return m.deploy_trigger_rollback()
	}
}

export function actorName(deploy: Deploy): string {
	return deploy.actor.name ?? m.deploy_actor_unknown()
}

/** "API · github-actions" */
export function triggerText(deploy: Deploy): string {
	return m.deploy_trigger_actor({
		trigger: triggerLabel(deploy.trigger),
		actor: actorName(deploy),
	})
}

/** "52 s", "1 min 4 s" (en el locale de la UI). */
export function durationText(ms: number | null, locale: string = getLocale()): string {
	if (ms === null) return m.deploy_none()
	const total = Math.max(0, Math.round(ms / 1000))
	const unit = (value: number, kind: 'second' | 'minute' | 'hour') =>
		new Intl.NumberFormat(locale, { style: 'unit', unit: kind, unitDisplay: 'short' }).format(
			value,
		)
	if (total < 60) return unit(total, 'second')
	const minutes = Math.floor(total / 60)
	if (minutes < 60) {
		const seconds = total % 60
		return seconds
			? `${unit(minutes, 'minute')} ${unit(seconds, 'second')}`
			: unit(minutes, 'minute')
	}
	const hours = Math.floor(minutes / 60)
	const rest = minutes % 60
	return rest ? `${unit(hours, 'hour')} ${unit(rest, 'minute')}` : unit(hours, 'hour')
}

/** Bytes en MB enteros ("297 MB"), como el diseño. */
export function megabytesText(bytes: number, locale: string = getLocale()): string {
	return new Intl.NumberFormat(locale, {
		style: 'unit',
		unit: 'megabyte',
		maximumFractionDigits: 0,
	}).format(bytes / (1024 * 1024))
}

/** El número de MB solo, para el valor grande ("297"). */
export function megabytesNumber(bytes: number, locale: string = getLocale()): string {
	return new Intl.NumberFormat(locale, { maximumFractionDigits: 0 }).format(bytes / (1024 * 1024))
}

/** "3,4" con un decimal. */
export function percentNumber(value: number, locale: string = getLocale()): string {
	return new Intl.NumberFormat(locale, {
		minimumFractionDigits: 1,
		maximumFractionDigits: 1,
	}).format(value)
}

/** "14:32:07" del log de un deploy (hora local). */
export function clockTime(value: Date, locale: string = getLocale()): string {
	return new Intl.DateTimeFormat(locale, {
		hour: '2-digit',
		minute: '2-digit',
		second: '2-digit',
		hour12: false,
	}).format(value)
}

/** "14:31:40.118" de los logs en vivo. */
export function clockTimeMs(value: Date, locale: string = getLocale()): string {
	return new Intl.DateTimeFormat(locale, {
		hour: '2-digit',
		minute: '2-digit',
		second: '2-digit',
		fractionalSecondDigits: 3,
		hour12: false,
	}).format(value)
}

/** El estado de una tarea como lo muestra el diseño: "Running · healthy". */
export function taskStateText(state: string, health: string | null): string {
	const capitalized = state.charAt(0).toUpperCase() + state.slice(1)
	return health ? `${capitalized} · ${health}` : capitalized
}

/** Un mensaje del backend (en minúscula, estilo Go) para mostrar como oración. */
export function sentence(text: string): string {
	return text.charAt(0).toUpperCase() + text.slice(1)
}

export function taskTone(state: string, health: string | null): StatusTone {
	if (state === 'failed' || state === 'rejected' || health === 'unhealthy') return 'bad'
	if (state === 'running' && (health === null || health === 'healthy')) return 'ok'
	if (state === 'shutdown' || state === 'complete' || state === 'orphaned') return 'idle'
	return 'warn'
}

/**
 * El healthcheck de una app de formulario, como lo muestra el Resumen ("GET /health · cada
 * 10 s"). null en una compose (cada servicio trae el suyo en el compose) y sin healthcheck
 * (`none`: vale el `HEALTHCHECK` de la imagen, si trae uno).
 */
export function healthcheckText(app: CatalogApp): string | null {
	if (app.kind !== 'service') return null
	const healthcheck = app.spec.healthcheck
	switch (healthcheck.type) {
		case 'none':
			return null
		case 'http':
		case 'bun-http':
			return m.deploy_healthcheck_http({
				path: healthcheck.path,
				interval: healthcheck.intervalSeconds,
			})
		case 'command':
			return m.deploy_healthcheck_command({
				command: healthcheck.command.join(' '),
				interval: healthcheck.intervalSeconds,
			})
	}
}
