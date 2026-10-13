import { http, HttpResponse } from 'msw'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { api, apiError } from '@/mocks/api'
import { authDb, MOCK_USERNAME } from '@/mocks/db/auth'
import { deployDb } from '@/mocks/db/deploy'
import { server } from '@/mocks/server'
import { pendingPhase, UnavailableError, ValidationError } from '@/shared/domain'
import { deployUseCases } from '../container'
import {
	type AppLogEvent,
	type AppRef,
	appHealth,
	asDeployId,
	type DeployLogEvent,
	DeployNotFound,
	LogStreamFailed,
	NoPreviousDeploy,
} from '../domain'
import { FetchEventSource } from './fetchEventSource'

// Recorre el módulo entero —caso de uso, repositorio, datasource (HTTP y SSE), schemas Zod
// y mappers— contra el backend simulado de src/mocks/handlers/deploy.ts.
const APP: AppRef = { project: 'auditfy', stage: 'staging', app: 'api' }
const APP_PATH = '/projects/:p/stages/:s/apps/:a'

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
beforeEach(() => {
	authDb.reset({ session: { username: MOCK_USERNAME, mfaPending: false, badCodes: 0 } })
	// Los deploys nuevos duran ~100 ms en vez de ~9 s; sin el #144 en curso de la siembra.
	deployDb.reset({ liveScale: 0.01, seedLive: false })
	vi.stubGlobal('EventSource', FetchEventSource)
})
afterEach(() => {
	server.resetHandlers()
	vi.unstubAllGlobals()
	for (const source of FetchEventSource.instances.splice(0)) source.close()
})
afterAll(() => server.close())

async function collect<T>(iterable: AsyncIterable<T>, limit = 10_000): Promise<T[]> {
	const items: T[] = []
	for await (const item of iterable) {
		items.push(item)
		if (items.length >= limit) break
	}
	return items
}

function logOf(events: readonly DeployLogEvent[]): string {
	return events.map((event) => (event.kind === 'chunk' ? event.text : '')).join('')
}

function statusesOf(events: readonly DeployLogEvent[]): string[] {
	return events.flatMap((event) => (event.kind === 'status' ? [event.status] : []))
}

describe('estado e historial', () => {
	it('el estado de una app sembrada: corriendo con sus réplicas y tareas', async () => {
		const status = await deployUseCases.getAppStatus(APP)

		expect(appHealth(status)).toBe('running')
		const [service] = status.services
		expect(service).toMatchObject({ name: 'api', desiredReplicas: 2, runningReplicas: 2 })
		// Solo se miden las tareas del nodo de muelle: la réplica 2 corre en worker-1.
		const [first, second] = service?.tasks ?? []
		expect(first?.cpuPercent).toBeGreaterThan(0)
		expect(first?.memoryBytes).toBeGreaterThan(0)
		expect(second?.node).toBe('worker-1')
		expect(second?.cpuPercent).toBeNull()
		expect(service?.memoryLimitBytes).toBe(512 * 1024 * 1024)
		expect(service?.tasks.map((task) => task.state)).toEqual([
			'running',
			'running',
			'shutdown',
			'failed',
		])
		expect(service?.tasks.at(-1)?.error).toBe('unhealthy container')
	})

	it('una app que nunca se desplegó no tiene servicios', async () => {
		const status = await deployUseCases.getAppStatus({ ...APP, app: 'nueva' })
		expect(appHealth(status)).toBe('not_deployed')
	})

	it('el historial del diseño, del más nuevo al más viejo', async () => {
		const list = await deployUseCases.listDeploys(APP)

		expect(list.map((deploy) => [deploy.id, deploy.status])).toEqual([
			[143, 'succeeded'],
			[142, 'failed'],
			[141, 'superseded'],
			[140, 'succeeded'],
			[139, 'succeeded'],
		])
		const failed = list[1]
		expect(failed?.error).toBe('la versión nueva no pasó el healthcheck')
		expect(failed?.actor).toEqual({ type: 'token', name: 'github-actions' })
		expect(list[0]?.images[0]?.digest).toMatch(/^sha256:[a-f0-9]{64}$/)
		expect(list[2]?.startedAt).toBeNull()
	})

	it('un deploy que no existe es DeployNotFound, no la fase', async () => {
		const result = deployUseCases.getDeploy(APP, asDeployId(9999))
		await expect(result).rejects.toBeInstanceOf(DeployNotFound)
		await expect(result).rejects.toSatisfy((error) => pendingPhase(error) === null)
	})
})

describe('desplegar y seguir el log', () => {
	it('por SSE: queued → running → succeeded, con el log entero', async () => {
		// Un poco más lento (~1 s) para que el stream alcance a verlo en cola.
		deployDb.reset({ liveScale: 0.1, seedLive: false })
		const { id } = await deployUseCases.createDeploy({
			app: APP,
			services: ['api'],
			overrides: {},
		})
		expect(id).toBe(144)

		const events = await collect(
			deployUseCases.followDeployLog(
				{ app: APP, id, live: true },
				new AbortController().signal,
			),
		)

		expect(statusesOf(events)).toEqual(['queued', 'running', 'succeeded'])
		expect(events.some((event) => event.kind === 'polling')).toBe(false)
		const log = logOf(events)
		expect(log).toContain('deploy #144 encolado · panel · felipe')
		expect(log).toContain('$ docker stack deploy')
		expect(log.trimEnd().split('\n').at(-1)).toMatch(/deploy exitoso en \d+ s$/)

		const deploy = await deployUseCases.getDeploy(APP, id)
		expect(deploy.status).toBe('succeeded')
		expect(deploy.trigger).toBe('panel')
	})

	it('una imagen rota termina failed por healthcheck, con el secreto enmascarado', async () => {
		const { id } = await deployUseCases.createDeploy({
			app: APP,
			services: ['api'],
			overrides: { api: 'ghcr.io/auditfy/api:broken' },
		})

		const events = await collect(
			deployUseCases.followDeployLog(
				{ app: APP, id, live: true },
				new AbortController().signal,
			),
		)

		expect(statusesOf(events).at(-1)).toBe('failed')
		expect(logOf(events)).toContain('postgres://auditfy:***@10.0.3.4')
		const deploy = await deployUseCases.getDeploy(APP, id)
		expect(deploy.error).toBe('la versión nueva no pasó el healthcheck')
	})

	it('si el SSE falla, sigue por polling ?offset hasta el final', async () => {
		server.use(http.get(api(`${APP_PATH}/deploys/:id/log/stream`), () => HttpResponse.error()))
		const { id } = await deployUseCases.createDeploy({
			app: APP,
			services: ['api'],
			overrides: {},
		})

		const events = await collect(
			deployUseCases.followDeployLog(
				{ app: APP, id, live: true },
				new AbortController().signal,
			),
		)

		expect(events[0]).toEqual({ kind: 'polling' })
		expect(statusesOf(events).at(-1)).toBe('succeeded')
		expect(logOf(events)).toContain('deploy exitoso')
	})

	it('sin EventSource en el entorno, también por polling', async () => {
		vi.unstubAllGlobals()
		const { id } = await deployUseCases.createDeploy({
			app: APP,
			services: ['api'],
			overrides: {},
		})

		const events = await collect(
			deployUseCases.followDeployLog(
				{ app: APP, id, live: true },
				new AbortController().signal,
			),
		)

		expect(events[0]).toEqual({ kind: 'polling' })
		expect(statusesOf(events).at(-1)).toBe('succeeded')
	})

	it('el log de un deploy terminado se lee entero por ?offset', async () => {
		const events = await collect(
			deployUseCases.followDeployLog(
				{ app: APP, id: asDeployId(142), live: false },
				new AbortController().signal,
			),
		)

		expect(FetchEventSource.instances).toEqual([])
		expect(statusesOf(events)).toEqual(['failed'])
		expect(logOf(events)).toContain('error: relation "findings_v2" does not exist')
	})

	it('un deploy nuevo reemplaza al que estaba en cola', async () => {
		deployDb.reset({ liveScale: 1, seedLive: false })
		const first = await deployUseCases.createDeploy({
			app: APP,
			services: ['api'],
			overrides: {},
		})
		const second = await deployUseCases.createDeploy({
			app: APP,
			services: ['api'],
			overrides: {},
		})

		const list = await deployUseCases.listDeploys(APP)
		expect(list.find((deploy) => deploy.id === first.id)?.status).toBe('superseded')
		expect(list.find((deploy) => deploy.id === second.id)?.status).toBe('queued')
	})

	it('un servicio que no existe en una compose es un 422 con su campo', async () => {
		const result = deployUseCases.createDeploy({
			app: APP,
			kind: 'compose',
			services: ['api'],
			overrides: { worker: 'ghcr.io/auditfy/worker:v1' },
		})
		await expect(result).rejects.toBeInstanceOf(ValidationError)
		await expect(result).rejects.toMatchObject({ field: 'images.worker' })
	})
})

describe('el mock valida la imagen como el backend', () => {
	// Directo contra el endpoint, sin el chequeo del panel: el 422 lo tiene que dar el mock.
	it.each([
		['Ghcr.io/Foo/Bar:Latest', 422],
		['a//b', 422],
		['a::b', 422],
		['a:b:c', 422],
		['a/../b', 422],
		['api', 202],
		['nginx:1.0-', 202],
	] as const)('%j → %i', async (image, expected) => {
		const response = await fetch(
			`http://localhost/api/v1/projects/auditfy/stages/staging/apps/api/deploys`,
			{
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ image }),
			},
		)
		expect(response.status).toBe(expected)
	})
})

describe('rollback', () => {
	it('vuelve a las imágenes del succeeded anterior al actual', async () => {
		const { id } = await deployUseCases.rollback(APP)

		const deploy = await deployUseCases.getDeploy(APP, id)
		expect(deploy.trigger).toBe('rollback')
		expect(deploy.images[0]?.ref).toBe('ghcr.io/auditfy/api:sha-e52b8f0')
	})

	it('sin un succeeded anterior es NoPreviousDeploy (409)', async () => {
		await expect(deployUseCases.rollback({ ...APP, app: 'nats' })).rejects.toBeInstanceOf(
			NoPreviousDeploy,
		)
	})
})

describe('logs en vivo', () => {
	it('trae las últimas N líneas por SSE, con tarea y nivel', async () => {
		const controller = new AbortController()
		const events: AppLogEvent[] = await collect(
			deployUseCases.streamAppLogs(
				{ app: APP, service: 'api', tail: 100 },
				controller.signal,
			),
			101,
		)
		controller.abort()

		expect(events[0]).toEqual({ kind: 'open' })
		const lines = events.flatMap((event) => (event.kind === 'line' ? [event.line] : []))
		expect(lines).toHaveLength(100)
		expect(new Set(lines.map((line) => line.task))).toEqual(new Set(['api.1', 'api.2']))
		expect(lines.some((line) => line.level === 'warn')).toBe(true)
		expect(lines.some((line) => line.level === 'error')).toBe(true)
		// El texto llega entero (el nivel solo da color) y sin escapes ANSI.
		expect(lines.some((line) => line.text.startsWith('WARN '))).toBe(true)
		expect(lines.every((line) => !line.text.includes(String.fromCharCode(27)))).toBe(true)
	})

	it('un servicio que no existe corta el stream antes de abrir', async () => {
		const result = collect(
			deployUseCases.streamAppLogs(
				{ app: APP, service: 'worker', tail: 100 },
				new AbortController().signal,
			),
		)
		await expect(result).rejects.toBeInstanceOf(LogStreamFailed)
		await expect(result).rejects.toMatchObject({ opened: false })
	})
})

describe('contra un backend sin la Fase 2', () => {
	it('una ruta que no existe (no_route) es UnavailableError con phase 2', async () => {
		server.use(
			http.get(api(`${APP_PATH}/status`), () =>
				apiError(404, 'no_route', 'no existe la ruta'),
			),
		)
		const result = deployUseCases.getAppStatus(APP)
		await expect(result).rejects.toBeInstanceOf(UnavailableError)
		await expect(result).rejects.toSatisfy((error) => pendingPhase(error) === 2)
	})

	it('un método que no existe (405) también', async () => {
		server.use(
			http.post(api(`${APP_PATH}/deploys`), () =>
				apiError(405, 'method_not_allowed', 'método no permitido'),
			),
		)
		await expect(
			deployUseCases.createDeploy({ app: APP, services: ['api'], overrides: {} }),
		).rejects.toSatisfy((error) => pendingPhase(error) === 2)
	})

	it('el rollback y el historial, igual', async () => {
		server.use(
			http.post(api(`${APP_PATH}/rollback`), () => apiError(404, 'no_route', 'no existe')),
			http.get(api(`${APP_PATH}/deploys`), () => apiError(404, 'no_route', 'no existe')),
		)
		await expect(deployUseCases.rollback(APP)).rejects.toSatisfy(
			(error) => pendingPhase(error) === 2,
		)
		await expect(deployUseCases.listDeploys(APP)).rejects.toSatisfy(
			(error) => pendingPhase(error) === 2,
		)
	})

	it('un 503 (Docker no responde) es UnavailableError sin fase: un error de verdad', async () => {
		server.use(
			http.get(api(`${APP_PATH}/status`), () =>
				apiError(503, 'docker_unavailable', 'Docker no responde'),
			),
		)
		const result = deployUseCases.getAppStatus(APP)
		await expect(result).rejects.toBeInstanceOf(UnavailableError)
		await expect(result).rejects.toSatisfy((error) => pendingPhase(error) === null)
	})
})
