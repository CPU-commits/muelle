import type {
	App,
	AppChanges,
	AppSlug,
	AppSpec,
	Healthcheck,
	NewApp,
	NewProject,
	Project,
	ProjectSlug,
	Stage,
	StageSlug,
} from '../../domain'
import { DEFAULT_HEALTHCHECK_TIMINGS } from '../../domain'
import type {
	AppDto,
	CreateAppPayload,
	CreateProjectPayload,
	HealthcheckDto,
	HealthcheckPayload,
	ProjectDto,
	SpecDto,
	SpecPayload,
	StageDto,
	UpdateAppPayload,
} from '../dtos/catalogDto'

// Los slugs que devuelve el backend ya pasaron su validación: se marcan sin volver a
// aplicar la regla (si la regla cambia allá, el panel no deja de mostrarlos).

export function projectFromDto(dto: ProjectDto): Project {
	return { slug: dto.slug as ProjectSlug, name: dto.name, createdAt: new Date(dto.created_at) }
}

export function stageFromDto(dto: StageDto): Stage {
	return {
		project: dto.project as ProjectSlug,
		slug: dto.slug as StageSlug,
		createdAt: new Date(dto.created_at),
	}
}

function healthcheckFromDto(dto: HealthcheckDto | null | undefined): Healthcheck {
	if (!dto || dto.type === 'none') return { type: 'none' }
	const timings = {
		intervalSeconds: dto.interval_s ?? DEFAULT_HEALTHCHECK_TIMINGS.intervalSeconds,
		timeoutSeconds: dto.timeout_s ?? DEFAULT_HEALTHCHECK_TIMINGS.timeoutSeconds,
		retries: dto.retries ?? DEFAULT_HEALTHCHECK_TIMINGS.retries,
		startPeriodSeconds: dto.start_period_s ?? DEFAULT_HEALTHCHECK_TIMINGS.startPeriodSeconds,
	}
	if (dto.type === 'command') return { type: 'command', command: dto.command ?? [], ...timings }
	return { type: dto.type, path: dto.path ?? '', ...timings }
}

// El backend guarda 0 como "sin límite" y lo omite (`omitzero`).
function positiveOrNull(value: number | undefined): number | null {
	return value && value > 0 ? value : null
}

export function specFromDto(dto: SpecDto): AppSpec {
	const memoryMb = positiveOrNull(dto.resources?.memory_mb)
	const cpus = positiveOrNull(dto.resources?.cpus)
	return {
		image: dto.image,
		port: dto.port,
		replicas: dto.replicas ?? 1,
		command: dto.command ?? [],
		healthcheck: healthcheckFromDto(dto.healthcheck),
		volumes: (dto.volumes ?? []).map((volume) => ({ name: volume.name, mount: volume.mount })),
		resources: memoryMb === null && cpus === null ? null : { memoryMb, cpus },
	}
}

export function appFromDto(dto: AppDto): App {
	const base = {
		project: dto.project as ProjectSlug,
		stage: dto.stage as StageSlug,
		slug: dto.slug as AppSlug,
		deployTimeoutSeconds: dto.deploy_timeout_s,
		configChangedAt: new Date(dto.config_changed_at),
		lastDeployedAt: dto.last_deployed_at ? new Date(dto.last_deployed_at) : null,
		pendingChanges: dto.pending_changes,
		createdAt: new Date(dto.created_at),
		services:
			dto.services?.map((service) => ({ name: service.name, ports: [...service.ports] })) ??
			null,
	}
	return dto.kind === 'service'
		? { ...base, kind: 'service', spec: specFromDto(dto.spec) }
		: { ...base, kind: 'compose', composeYaml: dto.compose_yaml }
}

// --- Cuerpos ------------------------------------------------------------------------------

export function createProjectPayload(input: NewProject): CreateProjectPayload {
	return { slug: input.slug, name: input.name }
}

function healthcheckPayload(healthcheck: Healthcheck): HealthcheckPayload {
	if (healthcheck.type === 'none') return { type: 'none' }
	const timings = {
		interval_s: healthcheck.intervalSeconds,
		timeout_s: healthcheck.timeoutSeconds,
		retries: healthcheck.retries,
		start_period_s: healthcheck.startPeriodSeconds,
	}
	if (healthcheck.type === 'command') {
		return { type: 'command', command: [...healthcheck.command], ...timings }
	}
	return { type: healthcheck.type, path: healthcheck.path, ...timings }
}

export function specPayload(spec: AppSpec): SpecPayload {
	const resources = spec.resources
	return {
		image: spec.image,
		port: spec.port,
		replicas: spec.replicas,
		command: [...spec.command],
		healthcheck: healthcheckPayload(spec.healthcheck),
		volumes: spec.volumes.map((volume) => ({ name: volume.name, mount: volume.mount })),
		resources: resources
			? {
					...(resources.memoryMb === null ? {} : { memory_mb: resources.memoryMb }),
					...(resources.cpus === null ? {} : { cpus: resources.cpus }),
				}
			: null,
	}
}

export function createAppPayload(input: NewApp): CreateAppPayload {
	return {
		slug: input.slug,
		kind: input.kind,
		...(input.kind === 'service'
			? { spec: specPayload(input.spec) }
			: { compose_yaml: input.composeYaml }),
		...(input.deployTimeoutSeconds === undefined
			? {}
			: { deploy_timeout_s: input.deployTimeoutSeconds }),
	}
}

export function updateAppPayload(changes: AppChanges): UpdateAppPayload {
	return {
		...(changes.spec ? { spec: specPayload(changes.spec) } : {}),
		...(changes.composeYaml === undefined ? {} : { compose_yaml: changes.composeYaml }),
		...(changes.deployTimeoutSeconds === undefined
			? {}
			: { deploy_timeout_s: changes.deployTimeoutSeconds }),
	}
}
