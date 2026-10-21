import { describe, expect, it } from 'vitest'
import {
	type AppSlug,
	type AppSpec,
	DEFAULT_HEALTHCHECK_TIMINGS,
	newServiceSpec,
	type ProjectSlug,
	type StageSlug,
} from '../../../domain'
import { appDtoSchema, type AppDto } from '../../dtos/catalogDto'
import {
	appFromDto,
	createAppPayload,
	projectFromDto,
	specFromDto,
	specPayload,
	stageFromDto,
	updateAppPayload,
} from '../catalogMapper'

const base = {
	project: 'auditfy',
	stage: 'staging',
	slug: 'api',
	deploy_timeout_s: 300,
	config_changed_at: '2026-10-10T12:05:00Z',
	last_deployed_at: '2026-10-10T12:00:00Z',
	pending_changes: true,
	created_at: '2026-09-01T00:00:00Z',
}

// El spec tal como lo guarda el backend (vo.AppSpec.JSON()).
const storedSpec = {
	image: 'ghcr.io/aumi-app/backend-app:sha-3f9c2e1',
	port: 3000,
	replicas: 2,
	command: [],
	healthcheck: {
		type: 'http',
		path: '/health',
		interval_s: 10,
		timeout_s: 5,
		retries: 3,
		start_period_s: 20,
	},
	volumes: [{ name: 'uploads', mount: '/app/uploads' }],
	resources: { memory_mb: 512 },
}

describe('catalogMapper', () => {
	it('traduce proyectos y stages', () => {
		expect(
			projectFromDto({ slug: 'auditfy', name: 'Auditfy', created_at: base.created_at }),
		).toEqual({ slug: 'auditfy', name: 'Auditfy', createdAt: new Date(base.created_at) })
		expect(
			stageFromDto({ project: 'auditfy', slug: 'staging', created_at: base.created_at }),
		).toEqual({ project: 'auditfy', slug: 'staging', createdAt: new Date(base.created_at) })
	})

	it('traduce una app de formulario con su spec', () => {
		const dto = appDtoSchema.parse({ ...base, kind: 'service', spec: storedSpec })

		expect(appFromDto(dto)).toEqual({
			kind: 'service',
			project: 'auditfy',
			stage: 'staging',
			slug: 'api',
			deployTimeoutSeconds: 300,
			configChangedAt: new Date(base.config_changed_at),
			lastDeployedAt: new Date(base.last_deployed_at),
			pendingChanges: true,
			createdAt: new Date(base.created_at),
			// Un backend sin la Fase 2 no manda `services` (contrato §2.12).
			services: null,
			spec: {
				image: 'ghcr.io/aumi-app/backend-app:sha-3f9c2e1',
				port: 3000,
				replicas: 2,
				command: [],
				healthcheck: { type: 'http', path: '/health', ...DEFAULT_HEALTHCHECK_TIMINGS },
				volumes: [{ name: 'uploads', mount: '/app/uploads' }],
				resources: { memoryMb: 512, cpus: null },
			},
		})
	})

	it('traduce una compose y una app sin deploys', () => {
		const dto = appDtoSchema.parse({
			...base,
			kind: 'compose',
			compose_yaml: 'services: {}\n',
			last_deployed_at: null,
			pending_changes: false,
		})

		expect(appFromDto(dto)).toMatchObject({
			kind: 'compose',
			composeYaml: 'services: {}\n',
			lastDeployedAt: null,
		})
	})

	it('los servicios de la app (contrato §2.12), con sus puertos', () => {
		const dto = appDtoSchema.parse({
			...base,
			kind: 'compose',
			compose_yaml: 'services: {}\n',
			services: [
				{ name: 'api', ports: [3000] },
				{ name: 'web', ports: [] },
			],
		})

		expect(appFromDto(dto).services).toEqual([
			{ name: 'api', ports: [3000] },
			{ name: 'web', ports: [] },
		])
	})

	it('una app de formulario sin spec no pasa el contrato', () => {
		expect(appDtoSchema.safeParse({ ...base, kind: 'service' }).success).toBe(false)
		expect(appDtoSchema.safeParse({ ...base, kind: 'compose' }).success).toBe(false)
	})

	it('completa lo que el backend omite: none, réplicas, sin límites', () => {
		expect(
			specFromDto({
				image: 'redis:7',
				port: 6379,
				replicas: 1,
				command: [],
				healthcheck: { type: 'none' },
				volumes: [],
				resources: null,
			}),
		).toEqual({ ...newServiceSpec('redis:7', 6379) })
		expect(
			specFromDto({
				image: 'x',
				port: 1,
				healthcheck: { type: 'command', command: ['true'] },
			}),
		).toMatchObject({
			replicas: 1,
			healthcheck: { type: 'command', command: ['true'], ...DEFAULT_HEALTHCHECK_TIMINGS },
			resources: null,
		})
	})

	it('el spec de ida y vuelta queda igual', () => {
		const dto = appDtoSchema.parse({ ...base, kind: 'service', spec: storedSpec }) as Extract<
			AppDto,
			{ kind: 'service' }
		>
		const spec = specFromDto(dto.spec)
		expect(specFromDto(specPayload(spec))).toEqual(spec)
		expect(specPayload(spec).resources).toEqual({ memory_mb: 512 })
	})

	it('arma los cuerpos con los nombres exactos del backend (json/v2 estricto)', () => {
		const spec: AppSpec = {
			...newServiceSpec('nginx', 80),
			healthcheck: { type: 'none' },
			resources: { memoryMb: null, cpus: 0.5 },
		}
		const where = {
			project: 'auditfy' as ProjectSlug,
			stage: 'staging' as StageSlug,
			slug: 'web' as AppSlug,
		}

		expect(createAppPayload({ ...where, kind: 'service', spec })).toEqual({
			slug: 'web',
			kind: 'service',
			spec: {
				image: 'nginx',
				port: 80,
				replicas: 1,
				command: [],
				healthcheck: { type: 'none' },
				volumes: [],
				resources: { cpus: 0.5 },
			},
		})
		expect(
			createAppPayload({
				...where,
				kind: 'compose',
				composeYaml: 'services: {}',
				deployTimeoutSeconds: 600,
			}),
		).toEqual({
			slug: 'web',
			kind: 'compose',
			compose_yaml: 'services: {}',
			deploy_timeout_s: 600,
		})
		expect(updateAppPayload({ deployTimeoutSeconds: 90 })).toEqual({ deploy_timeout_s: 90 })
		expect(updateAppPayload({ composeYaml: 'x' })).toEqual({ compose_yaml: 'x' })
		expect(Object.keys(updateAppPayload({ spec }))).toEqual(['spec'])
	})
})
