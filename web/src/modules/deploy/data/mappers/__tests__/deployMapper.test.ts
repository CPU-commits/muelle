import { describe, expect, it } from 'vitest'
import { appStatusDtoSchema, deployDtoSchema, logPageDtoSchema } from '../../dtos/deployDto'
import {
	appLogLineFromDto,
	appStatusFromDto,
	createDeployPayload,
	deployAcceptedFromDto,
	deployFromDto,
	logPageFromDto,
} from '../deployMapper'

const DIGEST = `sha256:${'9b2de1f4'.repeat(8)}`

describe('deployFromDto', () => {
	it('traduce snake_case, separa los digests y ordena las imágenes por servicio', () => {
		const dto = deployDtoSchema.parse({
			id: 143,
			status: 'succeeded',
			images: { worker: 'ghcr.io/x/worker:v1', api: `ghcr.io/x/api:sha-3f9c2e1@${DIGEST}` },
			trigger: 'api',
			actor_type: 'token',
			actor_name: 'github-actions',
			error: null,
			created_at: '2026-10-10T14:19:49Z',
			started_at: '2026-10-10T14:19:49.500Z',
			finished_at: '2026-10-10T14:20:41Z',
		})

		expect(deployFromDto(dto)).toEqual({
			id: 143,
			status: 'succeeded',
			images: [
				{ service: 'api', ref: 'ghcr.io/x/api:sha-3f9c2e1', digest: DIGEST },
				{ service: 'worker', ref: 'ghcr.io/x/worker:v1', digest: null },
			],
			trigger: 'api',
			actor: { type: 'token', name: 'github-actions' },
			error: null,
			createdAt: new Date('2026-10-10T14:19:49Z'),
			startedAt: new Date('2026-10-10T14:19:49.500Z'),
			finishedAt: new Date('2026-10-10T14:20:41Z'),
		})
	})

	it('los campos opcionales que faltan son null', () => {
		const dto = deployDtoSchema.parse({
			id: 145,
			status: 'queued',
			images: {},
			trigger: 'panel',
			actor_type: 'user',
			created_at: '2026-10-10T14:32:07Z',
		})
		expect(deployFromDto(dto)).toMatchObject({
			actor: { type: 'user', name: null },
			error: null,
			startedAt: null,
			finishedAt: null,
		})
	})

	it('el schema rechaza un estado que no es del spec', () => {
		expect(
			deployDtoSchema.safeParse({
				id: 1,
				status: 'done',
				images: {},
				trigger: 'panel',
				actor_type: 'user',
				created_at: '2026-10-10T14:32:07Z',
			}).success,
		).toBe(false)
	})
})

describe('appStatusFromDto', () => {
	it('traduce servicios y tareas', () => {
		const dto = appStatusDtoSchema.parse({
			services: [
				{
					name: 'api',
					replicas_desired: 2,
					replicas_running: 1,
					cpu_percent: 3.4,
					memory_bytes: 1024,
					memory_limit_bytes: 2048,
					tasks: [
						{
							id: 'k3x9q2m',
							slot: 1,
							state: 'running',
							desired_state: 'running',
							health: 'healthy',
							node: 'manager-1',
							since: '2026-10-10T14:20:05Z',
							error: '',
						},
						{ id: 'c7f4e0b', state: 'failed', desired_state: 'shutdown' },
					],
				},
			],
		})

		expect(appStatusFromDto(dto)).toEqual({
			services: [
				{
					name: 'api',
					desiredReplicas: 2,
					runningReplicas: 1,
					cpuPercent: 3.4,
					memoryBytes: 1024,
					memoryLimitBytes: 2048,
					tasks: [
						{
							id: 'k3x9q2m',
							slot: 1,
							state: 'running',
							desiredState: 'running',
							health: 'healthy',
							node: 'manager-1',
							since: new Date('2026-10-10T14:20:05Z'),
							error: null,
						},
						{
							id: 'c7f4e0b',
							slot: null,
							state: 'failed',
							desiredState: 'shutdown',
							health: null,
							node: null,
							since: null,
							error: null,
						},
					],
				},
			],
		})
	})
})

describe('log', () => {
	it('logPageFromDto pasa el offset siguiente', () => {
		const dto = logPageDtoSchema.parse({ chunk: 'a\n', next_offset: 2, status: 'running' })
		expect(logPageFromDto(dto)).toEqual({ chunk: 'a\n', nextOffset: 2, status: 'running' })
	})

	it('appLogLineFromDto saca el nivel del texto', () => {
		expect(
			appLogLineFromDto({
				time: '2026-10-10T14:31:51.517Z',
				task: 'api.2',
				stream: 'stdout',
				line: 'WARN consulta lenta',
			}),
		).toEqual({
			time: new Date('2026-10-10T14:31:51.517Z'),
			task: 'api.2',
			stream: 'stdout',
			text: 'consulta lenta',
			level: 'warn',
		})
	})
})

describe('payloads', () => {
	it('createDeployPayload arma el cuerpo de cada caso del spec §7', () => {
		expect(createDeployPayload({ kind: 'redeploy' })).toEqual({})
		expect(createDeployPayload({ kind: 'image', image: 'x:1' })).toEqual({ image: 'x:1' })
		expect(createDeployPayload({ kind: 'images', images: { web: 'x:2' } })).toEqual({
			images: { web: 'x:2' },
		})
	})

	it('deployAcceptedFromDto se queda con el id', () => {
		expect(deployAcceptedFromDto({ id: 145, status_url: '/api/v1/…/deploys/145' })).toEqual({
			id: 145,
		})
	})
})
