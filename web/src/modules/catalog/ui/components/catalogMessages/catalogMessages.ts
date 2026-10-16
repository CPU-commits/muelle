import { m } from '@/shared/ui/i18n/paraglide/messages'
import { describeError } from '@/shared/ui'
import { ValidationError } from '@/shared/domain'
import {
	type AppKind,
	type AppSpecMessages,
	type ComposeServiceSummary,
	AppDeployStopping,
	AppHasDomains,
	AppNotFound,
	InvalidCompose,
	InvalidDeployTimeout,
	InvalidProjectName,
	InvalidSlug,
	KindMismatch,
	PROJECT_NAME_MAX,
	ProjectNotFound,
	SlugTaken,
	StageNetworkConflict,
	StageNotFound,
} from '../../../domain'

// Los textos del catálogo que dependen de un dato (plurales, tipo de app) o de un error.
// La pantalla elige el mensaje por la CLASE del error de dominio, nunca por un status.

export function appsCount(count: number): string {
	return count === 1 ? m.catalog_apps_one() : m.catalog_apps_other({ count })
}

export function stagesCount(count: number): string {
	return count === 1 ? m.catalog_stages_one() : m.catalog_stages_other({ count })
}

export function kindLabel(kind: AppKind): string {
	return kind === 'service' ? m.catalog_app_kind_service() : m.catalog_app_kind_compose()
}

/** "2 réplicas · con healthcheck · volumen workspace, stop-first" (aside de App·Compose). */
export function composeServiceDetails(service: ComposeServiceSummary): string {
	const replicas = service.replicas ?? 1
	const parts = [
		replicas === 1
			? m.catalog_compose_replicas_one()
			: m.catalog_compose_replicas_other({ count: replicas }),
		service.healthcheck === 'defined'
			? m.catalog_compose_with_healthcheck()
			: m.catalog_compose_no_downtime_warning(),
	]
	if (service.volumes.length > 0) {
		const names = service.volumes.join(', ')
		parts.push(
			service.volumes.length === 1
				? m.catalog_compose_volumes_one({ names })
				: m.catalog_compose_volumes_other({ names }),
		)
	}
	return parts.join(' · ')
}

/** El aviso de un servicio sin healthcheck, o null. */
export function composeServiceWarning(service: ComposeServiceSummary): string | null {
	if (service.healthcheck === 'missing') return m.catalog_compose_no_healthcheck()
	if (service.healthcheck === 'disabled') return m.catalog_compose_healthcheck_disabled()
	return null
}

/** Los textos de las reglas del spec en el idioma de la UI (los del dominio son los del backend). */
export const specMessages: AppSpecMessages = {
	imageRequired: () => m.catalog_spec_image_required(),
	imageInvalid: () => m.catalog_spec_image_invalid(),
	port: () => m.catalog_spec_port(),
	replicas: () => m.catalog_spec_replicas(),
	commandTooLong: () => m.catalog_spec_command_long(),
	healthcheckPath: () => m.catalog_spec_health_path(),
	healthcheckCommand: () => m.catalog_spec_health_command(),
	interval: () => m.catalog_spec_interval(),
	timeout: () => m.catalog_spec_timeout(),
	retries: () => m.catalog_spec_retries(),
	startPeriod: () => m.catalog_spec_start_period(),
	volumesTooMany: () => m.catalog_spec_volumes_many(),
	volumeName: () => m.catalog_spec_volume_name(),
	volumeMount: () => m.catalog_spec_volume_mount(),
	volumeNameRepeated: (name) => m.catalog_spec_volume_name_repeated({ name }),
	volumeMountRepeated: (mount) => m.catalog_spec_volume_mount_repeated({ mount }),
	memory: () => m.catalog_spec_memory(),
	cpus: () => m.catalog_spec_cpus(),
}

export function slugMessage(error: InvalidSlug, slug: string): string {
	return error.problem === 'reserved'
		? m.catalog_error_slug_reserved({ slug })
		: m.catalog_error_slug_format({ max: error.max })
}

export function projectNameMessage(error: InvalidProjectName): string {
	switch (error.problem) {
		case 'empty':
			return m.common_required()
		case 'long':
			return m.catalog_error_name_long({ max: PROJECT_NAME_MAX })
		case 'invisible':
			return m.catalog_error_name_invisible()
	}
}

export function slugTakenMessage(error: SlugTaken): string {
	switch (error.entity) {
		case 'project':
			return m.catalog_error_project_taken()
		case 'stage':
			return m.catalog_error_stage_taken()
		case 'app':
			return m.catalog_error_app_taken()
	}
}

// El texto de algo puesto entre comillas en el mensaje del backend (`"uploads" está repetido`).
function quoted(message: string, index = 0): string {
	return [...message.matchAll(/"([^"]*)"/g)][index]?.[1] ?? ''
}

/** Un motivo del backend (en minúscula, estilo Go) como oración. */
function sentence(text: string): string {
	return text.charAt(0).toUpperCase() + text.slice(1)
}

const TIMINGS: Readonly<Record<string, () => string>> = {
	'healthcheck.intervalSeconds': () => specMessages.interval(),
	'healthcheck.timeoutSeconds': () => specMessages.timeout(),
	'healthcheck.retries': () => specMessages.retries(),
	'healthcheck.startPeriodSeconds': () => specMessages.startPeriod(),
}

/**
 * El texto traducido de un 422 del backend para un campo (con el nombre del dominio). El
 * backend solo habla español y nombra el campo ("healthcheck.interval_s: debe estar…"): el
 * panel muestra su propio texto, sin el nombre del campo. Lo que no reconoce, el motivo
 * tal cual.
 */
export function serverFieldMessage(field: string, backend: string): string {
	const repeated = /repetid/.test(backend)
	const timing = TIMINGS[field]
	if (timing) return timing()
	if (/^volumes\[\d+\]\.name$/.test(field)) {
		return repeated
			? specMessages.volumeNameRepeated(quoted(backend))
			: specMessages.volumeName()
	}
	if (/^volumes\[\d+\]\.mount$/.test(field)) {
		return repeated
			? specMessages.volumeMountRepeated(quoted(backend))
			: specMessages.volumeMount()
	}
	switch (field) {
		case 'image':
			return /obligatoria/.test(backend)
				? specMessages.imageRequired()
				: specMessages.imageInvalid()
		case 'port':
			return specMessages.port()
		case 'replicas':
			return specMessages.replicas()
		case 'command':
			return specMessages.commandTooLong()
		case 'healthcheck':
		case 'healthcheck.type':
			return m.catalog_error_healthcheck_shape()
		// El backend usa el mismo campo para el formato y para "no va con este tipo"
		// (appspec.go): el texto del formato solo cuando el motivo es el formato.
		case 'healthcheck.path':
			return /empezar con/.test(backend)
				? specMessages.healthcheckPath()
				: m.catalog_error_healthcheck_shape()
		case 'healthcheck.command':
			return /obligatorio/.test(backend)
				? specMessages.healthcheckCommand()
				: m.catalog_error_healthcheck_shape()
		case 'volumes':
			return specMessages.volumesTooMany()
		case 'resources':
		case 'resources.memoryMb':
			return specMessages.memory()
		case 'resources.cpus':
			return specMessages.cpus()
		case 'deployTimeoutSeconds':
			return m.catalog_error_timeout()
		// Al guardar, catalog valida el compose con las reglas de §6.3 del diseño (contrato
		// §2.12): el motivo del backend nombra la clave (`services.web.ports: …`) y dice por
		// qué; el panel no tiene un texto propio para cada regla.
		case 'composeYaml':
			if (/^es obligatorio/.test(backend)) return m.catalog_error_compose_empty()
			if (/256 KiB/.test(backend)) return m.catalog_error_compose_too_large()
			if (/ya existe en la app/.test(backend)) {
				return m.catalog_error_compose_service_taken({
					service: quoted(backend),
					app: quoted(backend, 1),
				})
			}
			return sentence(backend)
		case 'spec':
			return m.catalog_error_spec_shape()
		case 'slug':
			if (/reservad/.test(backend))
				return m.catalog_error_slug_reserved({ slug: quoted(backend) })
			// Una app de formulario nueva con el nombre de un servicio de una compose del stage.
			if (/ya es un servicio/.test(backend)) {
				return m.catalog_error_slug_is_service({
					slug: quoted(backend),
					app: quoted(backend, 1),
				})
			}
			return m.catalog_error_slug_invalid()
		case 'name':
			if (/superar/.test(backend)) return m.catalog_error_name_long({ max: PROJECT_NAME_MAX })
			if (/obligatorio/.test(backend)) return m.common_required()
			return m.catalog_error_name_invisible()
		default:
			return backend
	}
}

/** Un error que no va en un campo: arriba del formulario o en un toast. */
export function catalogErrorMessage(error: unknown): string {
	if (error instanceof KindMismatch) return m.catalog_error_kind_mismatch()
	if (error instanceof ProjectNotFound) return m.catalog_error_project_not_found()
	if (error instanceof StageNotFound) return m.catalog_error_stage_not_found()
	if (error instanceof AppNotFound) return m.catalog_error_app_not_found()
	if (error instanceof SlugTaken) return slugTakenMessage(error)
	if (error instanceof AppHasDomains) return m.catalog_error_app_has_domains()
	if (error instanceof AppDeployStopping) return m.catalog_error_app_deploy_stopping()
	if (error instanceof StageNetworkConflict) return m.catalog_error_stage_network_conflict()
	if (error instanceof InvalidProjectName) return projectNameMessage(error)
	if (error instanceof InvalidSlug) return slugMessage(error, error.slug)
	if (error instanceof InvalidCompose) {
		return error.problem === 'empty'
			? m.catalog_error_compose_empty()
			: m.catalog_error_compose_too_large()
	}
	if (error instanceof InvalidDeployTimeout) return m.catalog_error_timeout()
	// Un 422 del backend en un campo que la pantalla no tiene a la vista: su texto, sin el
	// nombre del campo.
	if (error instanceof ValidationError) {
		return error.field ? serverFieldMessage(error.field, error.message) : error.message
	}
	return describeError(error)
}
