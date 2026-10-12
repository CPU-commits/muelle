import { m } from '@/shared/ui/i18n/paraglide/messages'
import { UnavailableError, ValidationError } from '@/shared/domain'
import {
	type AppKind,
	type AppSpecMessages,
	type ComposeServiceSummary,
	AppNotFound,
	InvalidCompose,
	InvalidDeployTimeout,
	InvalidProjectName,
	InvalidSlug,
	KindMismatch,
	PROJECT_NAME_MAX,
	ProjectNotFound,
	SlugTaken,
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

/** Un error que no va en un campo: arriba del formulario o en un toast. */
export function catalogErrorMessage(error: unknown): string {
	if (error instanceof KindMismatch) return m.catalog_error_kind_mismatch()
	if (error instanceof ProjectNotFound) return m.catalog_error_project_not_found()
	if (error instanceof StageNotFound) return m.catalog_error_stage_not_found()
	if (error instanceof AppNotFound) return m.catalog_error_app_not_found()
	if (error instanceof SlugTaken) return slugTakenMessage(error)
	if (error instanceof InvalidProjectName) return projectNameMessage(error)
	if (error instanceof InvalidSlug) return m.catalog_error_slug_format({ max: error.max })
	if (error instanceof InvalidCompose) {
		return error.problem === 'empty'
			? m.catalog_error_compose_empty()
			: m.catalog_error_compose_too_large()
	}
	if (error instanceof InvalidDeployTimeout) return m.catalog_error_timeout()
	// Un 422 del backend que el panel no esperaba: su motivo, que ya viene en español.
	if (error instanceof ValidationError) {
		return error.field ? `${error.field}: ${error.message}` : error.message
	}
	if (error instanceof UnavailableError) return m.common_unavailable()
	return m.common_error_generic()
}
