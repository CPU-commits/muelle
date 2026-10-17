import { http, HttpResponse } from 'msw'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { api, apiError } from '@/mocks/api'
import { authDb, MOCK_USERNAME } from '@/mocks/db/auth'
import { catalogDb } from '@/mocks/db/catalog'
import { deployDb } from '@/mocks/db/deploy'
import { appKey, ingressDb } from '@/mocks/db/ingress'
import { server } from '@/mocks/server'
import { appRefFrom, catalogUseCases } from '@/modules/catalog'
import { http as client } from '@/shared/data/http/client'
import { ApiError } from '@/shared/data/http/errors'
import {
	ConflictError,
	pendingPhase,
	UnauthorizedError,
	UnavailableError,
	ValidationError,
} from '@/shared/domain'
import { deployUseCases } from '../container'
import { httpDeployRepository } from '../data/repositories/httpDeployRepository'
import {
	type AppLogEvent,
	type AppRef,
	AppRemoving,
	appHealth,
	asDeployId,
	asLogOffset,
	type DeployLogEvent,
	DeployNotFound,
	DockerUnavailable,
	ImageRejected,
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
	// El mock de deploy lee el kind y los servicios de cada app del catálogo (contrato §2.2).
	catalogDb.reset()
	// Los deploys nuevos duran ~100 ms en vez de ~9 s; sin el #144 en curso de la siembra.
	deployDb.reset({ liveScale: 0.01, seedLive: false })
	ingressDb.reset()
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

	it('si Caddy no acepta la configuración, el paso 7 es una advertencia sin el error (contrato §2.11)', async () => {
		ingressDb.create(appKey('auditfy', 'staging', 'api'), {
			hostname: 'caddy-rechaza.tudominio.cl',
			targetService: 'api',
			targetPort: 3000,
			proxied: false,
		})
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

		expect(statusesOf(events)).toContain('succeeded')
		const log = logOf(events)
		expect(log).toContain(
			'advertencia: no se pudo actualizar Caddy; el motivo está en el estado del proxy (GET /api/v1/proxy)\n',
		)
		expect(log).not.toContain('rutas de Caddy actualizadas')
		// El motivo no va en el log (lo lee un token del proyecto).
		expect(log).not.toContain('caddy-rechaza')
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

describe('una app compose del catálogo (contrato §2.2 y §2.12)', () => {
	// aumi-agents/staging/agents: gateway, runner y scheduler, y nunca se desplegó.
	const COMPOSE: AppRef = { project: 'aumi-agents', stage: 'staging', app: 'agents' }
	const SERVICES = ['gateway', 'runner', 'scheduler']

	async function finish(app: AppRef, id: Parameters<typeof deployUseCases.getDeploy>[1]) {
		const events = await collect(
			deployUseCases.followDeployLog({ app, id, live: true }, new AbortController().signal),
		)
		return statusesOf(events).at(-1)
	}

	it('acepta images de un servicio del compose aunque nunca se desplegó', async () => {
		const { id } = await deployUseCases.createDeploy({
			app: COMPOSE,
			kind: 'compose',
			services: SERVICES,
			overrides: { gateway: 'ghcr.io/aumi-app/agents-gateway:v2' },
		})

		expect(await finish(COMPOSE, id)).toBe('succeeded')
		// El override para gateway; los otros, la ref del compose (primer deploy).
		const deploy = await deployUseCases.getDeploy(COMPOSE, id)
		expect(deploy.images.map((image) => [image.service, image.ref])).toEqual([
			['gateway', 'ghcr.io/aumi-app/agents-gateway:v2'],
			['runner', 'ghcr.io/aumi-app/agents-runner:staging'],
			['scheduler', 'ghcr.io/aumi-app/agents-scheduler:staging'],
		])
		// Swarm corre lo que se desplegó: el estado tiene los tres servicios.
		const status = await deployUseCases.getAppStatus(COMPOSE)
		expect(status.services.map((service) => service.name)).toEqual(SERVICES)
	})

	it('un servicio que no está en el compose es 422 images.<servicio>', async () => {
		const result = deployUseCases.createDeploy({
			app: COMPOSE,
			kind: 'compose',
			services: SERVICES,
			overrides: { web: 'ghcr.io/aumi-app/web:v1' },
		})
		await expect(result).rejects.toBeInstanceOf(ValidationError)
		await expect(result).rejects.toMatchObject({ field: 'images.web' })
	})

	it('image (el de una app de formulario) es 422 image', async () => {
		const response = await fetch(
			'http://localhost/api/v1/projects/aumi-agents/stages/staging/apps/agents/deploys',
			{
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ image: 'ghcr.io/aumi-app/agents-gateway:v2' }),
			},
		)
		expect(response.status).toBe(422)
		expect(await response.json()).toEqual({
			error: {
				code: 'invalid',
				message: 'image: una app compose recibe images, por servicio',
			},
		})
	})

	it('una compose ya desplegada tiene en el estado los servicios del compose', async () => {
		const status = await deployUseCases.getAppStatus({ ...COMPOSE, stage: 'production' })
		expect(status.services.map((service) => service.name)).toEqual(SERVICES)
	})

	it('los logs son de un servicio que corre en Swarm: uno de la compose, sí; el slug de la app, no', async () => {
		const production = { ...COMPOSE, stage: 'production' }
		const controller = new AbortController()
		const events = await collect(
			deployUseCases.streamAppLogs(
				{ app: production, service: 'runner', tail: 100 },
				controller.signal,
			),
			101,
		)
		controller.abort()
		const tasks = events.flatMap((event) => (event.kind === 'line' ? [event.line.task] : []))
		expect(new Set(tasks)).toEqual(new Set(['runner.1', 'runner.2']))

		const missing = collect(
			deployUseCases.streamAppLogs(
				{ app: production, service: 'agents', tail: 100 },
				new AbortController().signal,
			),
		)
		await expect(missing).rejects.toMatchObject({ opened: false })
	})
})

describe('qué imagen se despliega sin override (contrato §2.2)', () => {
	it('la del deploy actual, salvo que la configuración cambió desde ese deploy', async () => {
		const { id } = await deployUseCases.createDeploy({
			app: APP,
			services: ['api'],
			overrides: {},
		})
		await collect(
			deployUseCases.followDeployLog(
				{ app: APP, id, live: true },
				new AbortController().signal,
			),
		)
		expect((await deployUseCases.getDeploy(APP, id)).images[0]?.ref).toBe(
			'ghcr.io/auditfy/api:sha-3f9c2e1',
		)

		// Editar spec.image en Configuración: el próximo deploy sin override usa la nueva.
		const app = catalogDb.findApp('auditfy', 'staging', 'api')
		if (app?.spec) app.spec = { ...app.spec, image: 'ghcr.io/aumi-app/backend-app:v9' }
		const next = await deployUseCases.createDeploy({
			app: APP,
			services: ['api'],
			overrides: {},
		})
		await collect(
			deployUseCases.followDeployLog(
				{ app: APP, id: next.id, live: true },
				new AbortController().signal,
			),
		)
		expect((await deployUseCases.getDeploy(APP, next.id)).images[0]?.ref).toBe(
			'ghcr.io/aumi-app/backend-app:v9',
		)
	})
})

describe('el catálogo sigue a los deploys (diseño §7 paso 8)', () => {
	it('un deploy que termina bien adelanta last_deployed_at y saca los cambios sin desplegar', async () => {
		const ref = appRefFrom('auditfy', 'staging', 'api')
		// La api de staging tiene un cambio de configuración de después de #143.
		expect((await catalogUseCases.getApp(ref)).pendingChanges).toBe(true)

		const { id } = await deployUseCases.createDeploy({
			app: APP,
			services: ['api'],
			overrides: {},
		})
		await collect(
			deployUseCases.followDeployLog(
				{ app: APP, id, live: true },
				new AbortController().signal,
			),
		)

		const deploy = await deployUseCases.getDeploy(APP, id)
		const app = await catalogUseCases.getApp(ref)
		expect(app.lastDeployedAt).toEqual(deploy.startedAt)
		expect(app.pendingChanges).toBe(false)
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

	it('con since trae todo lo escrito desde ahí, inclusive, y nada de antes', async () => {
		const first = new AbortController()
		const before = (
			await collect(
				deployUseCases.streamAppLogs({ app: APP, service: 'api', tail: 100 }, first.signal),
				101,
			)
		).flatMap((event) => (event.kind === 'line' ? [event.line] : []))
		first.abort()
		const since = before.at(-3)?.time
		if (!since) throw new Error('sin hora')

		const second = new AbortController()
		const after = (
			await collect(
				deployUseCases.streamAppLogs(
					{ app: APP, service: 'api', tail: 100, since },
					second.signal,
				),
				4,
			)
		).flatMap((event) => (event.kind === 'line' ? [event.line] : []))
		second.abort()

		// La línea de `since` vuelve (inclusivo: el panel la descarta) y siguen las que venían.
		expect(after.map((line) => line.time?.getTime())).toEqual(
			before.slice(-3).map((line) => line.time?.getTime()),
		)
		expect(after.map((line) => line.text)).toEqual(before.slice(-3).map((line) => line.text))
	})

	it('si el stream no abre porque la sesión venció, es UnauthorizedError (y no se reintenta)', async () => {
		authDb.reset()
		const result = collect(
			deployUseCases.streamAppLogs(
				{ app: APP, service: 'api', tail: 100 },
				new AbortController().signal,
			),
		)
		await expect(result).rejects.toBeInstanceOf(UnauthorizedError)
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

/** Una respuesta SSE armada a mano: los eventos y cierra. */
function sseBody(events: readonly string[]): Response {
	return new Response(`${events.join('\n\n')}\n\n`, {
		headers: { 'content-type': 'text/event-stream' },
	})
}

describe('contrato de la Fase 2', () => {
	it('el estado trae la imagen y el estado de la actualización de cada servicio', async () => {
		const status = await deployUseCases.getAppStatus(APP)
		const api = status.services[0]
		expect(api?.image).toMatch(/^ghcr\.io\/.+@sha256:[0-9a-f]{64}$/)
		expect(api?.updateState).toBe('completed')
		// CPU, RAM y health por tarea: la del otro nodo, sin medir.
		const remote = api?.tasks.find((task) => task.node === 'worker-1')
		expect(remote?.cpuPercent).toBeNull()
		expect(remote?.memoryBytes).toBeNull()
		expect(remote?.health).toBeNull()
	})

	it('Docker caído (503 docker_unavailable) es DockerUnavailable, con su texto', async () => {
		server.use(
			http.get(api(`${APP_PATH}/status`), () =>
				apiError(503, 'docker_unavailable', 'Docker no responde'),
			),
		)
		await expect(deployUseCases.getAppStatus(APP)).rejects.toBeInstanceOf(DockerUnavailable)
	})

	it('el rollback sin anterior es solo no_previous_deploy: un conflict es la app que se borra', async () => {
		server.use(
			http.post(api(`${APP_PATH}/rollback`), () =>
				apiError(409, 'conflict', 'la app se está borrando'),
			),
		)
		const result = deployUseCases.rollback(APP)
		await expect(result).rejects.toBeInstanceOf(AppRemoving)
		await expect(result).rejects.toBeInstanceOf(ConflictError)
		await expect(result).rejects.not.toBeInstanceOf(NoPreviousDeploy)
	})

	it('un deploy de una app que se está borrando (409 conflict) es AppRemoving', async () => {
		server.use(
			http.post(api(`${APP_PATH}/deploys`), () =>
				apiError(409, 'conflict', 'la app se está borrando'),
			),
		)
		const result = deployUseCases.createDeploy({
			app: APP,
			kind: 'service',
			services: ['api'],
			overrides: {},
		})
		await expect(result).rejects.toBeInstanceOf(AppRemoving)
	})

	it('el log por polling: un offset más allá del final es 422 offset', async () => {
		const result = httpDeployRepository.readLog(APP, asDeployId(143), asLogOffset(10_000_000))
		await expect(result).rejects.toBeInstanceOf(ValidationError)
		await expect(result).rejects.toMatchObject({ field: 'offset' })
	})

	it('el log por polling trae superseded_by en un deploy reemplazado', async () => {
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
		const page = await httpDeployRepository.readLog(APP, first.id, asLogOffset(0))
		expect(page.status).toBe('superseded')
		expect(page.supersededBy).toBe(second.id)
	})

	it('el SSE de un deploy terminado manda las líneas y después el status final, sin uno al conectar', async () => {
		const events = await collect(
			httpDeployRepository.streamLog(
				APP,
				asDeployId(143),
				asLogOffset(0),
				new AbortController().signal,
			),
		)
		expect(events[0]?.kind).toBe('chunk')
		expect(events.filter((event) => event.kind === 'status')).toEqual([
			{ kind: 'status', status: 'succeeded' },
		])
		expect(events.at(-1)).toEqual({ kind: 'status', status: 'succeeded' })
	})

	it('el SSE de un deploy en curso empieza con su status', async () => {
		// A escala real (~9 s): a 0.01 dura ~100 ms y, con la máquina cargada, ya terminó
		// cuando se abre el stream (y un deploy terminado empieza con el log, no el status).
		deployDb.reset({ liveScale: 1, seedLive: false })
		const { id } = await deployUseCases.createDeploy({
			app: APP,
			services: ['api'],
			overrides: {},
		})
		const controller = new AbortController()
		const events = await collect(
			httpDeployRepository.streamLog(APP, id, asLogOffset(0), controller.signal),
			1,
		)
		controller.abort()
		// El primero es su estado (en cola o, con la máquina cargada, ya corriendo).
		expect(events[0]?.kind).toBe('status')
		expect(['queued', 'running']).toContain(
			events[0]?.kind === 'status' ? events[0].status : null,
		)
	})

	it('los logs terminan con `end`: eof (el servicio dejó de existir) y sin error', async () => {
		server.use(
			http.get(api(`${APP_PATH}/logs`), () =>
				sseBody([
					'event: log\ndata: {"time":"2026-10-10T14:31:40.118Z","task":"api.1","stream":"stdout","line":"última"}',
					'event: end\ndata: eof',
				]),
			),
		)
		const events = await collect(
			deployUseCases.streamAppLogs(
				{ app: APP, service: 'api', tail: 100 },
				new AbortController().signal,
			),
		)
		expect(events.map((event) => event.kind)).toEqual(['open', 'line', 'end'])
		expect(events.at(-1)).toEqual({ kind: 'end', reason: 'eof' })
	})

	it('los logs que llegan a su hora de vida terminan con `end` timeout', async () => {
		server.use(http.get(api(`${APP_PATH}/logs`), () => sseBody(['event: end\ndata: timeout'])))
		const events = await collect(
			deployUseCases.streamAppLogs(
				{ app: APP, service: 'api', tail: 100 },
				new AbortController().signal,
			),
		)
		expect(events.at(-1)).toEqual({ kind: 'end', reason: 'timeout' })
	})

	it('al reconectar manda since en vez de tail', async () => {
		const urls: URL[] = []
		server.use(
			http.get(api(`${APP_PATH}/logs`), ({ request }) => {
				urls.push(new URL(request.url))
				return sseBody(['event: end\ndata: eof'])
			}),
		)
		await collect(
			deployUseCases.streamAppLogs(
				{
					app: APP,
					service: 'api',
					tail: 500,
					since: new Date('2026-10-10T14:31:51.517Z'),
				},
				new AbortController().signal,
			),
		)
		expect(urls[0]?.searchParams.get('since')).toBe('2026-10-10T14:31:51.517Z')
		expect(urls[0]?.searchParams.has('tail')).toBe(false)
		expect(urls[0]?.searchParams.get('follow')).toBe('1')
	})

	it('el SSE del log responde 422 offset (antes de abrir) con un offset que pasa el log', async () => {
		const error = await client
			.get(`/projects/auditfy/stages/staging/apps/api/deploys/143/log/stream`, {
				query: { offset: 10_000_000 },
			})
			.catch((caught: unknown) => caught)
		expect(error).toBeInstanceOf(ApiError)
		expect((error as ApiError).status).toBe(422)
		expect((error as ApiError).message).toMatch(/^offset: /)
	})

	it('con since, el mock no mira tail', async () => {
		const controller = new AbortController()
		const events = await collect(
			deployUseCases.streamAppLogs(
				{ app: APP, service: 'api', tail: 100, since: new Date(Date.now() - 5000) },
				controller.signal,
			),
			1,
		)
		controller.abort()
		expect(events[0]).toEqual({ kind: 'open' })
		// Un tail fuera de rango junto a since no es 422: con since, tail no se mira.
		const result = await client
			.get(`/projects/auditfy/stages/staging/apps/api/logs`, {
				query: { service: 'api', since: new Date().toISOString(), tail: 99_999, follow: 0 },
			})
			.catch((caught: unknown) => caught)
		expect(result).not.toMatchObject({ status: 422 })
	})

	it.each([
		[{ service: 'api', follow: 2 }, 'follow'],
		[{ service: 'api', follow: 'si' }, 'follow'],
		// La query se valida antes de buscar el servicio en Swarm (contrato §2.9).
		[{ service: 'noexiste', tail: 99_999 }, 'tail'],
		[{ service: 'noexiste', since: 'ayer' }, 'since'],
		[{ service: 'NoEsUnSlug' }, 'service'],
	] as const)('el mock valida la query como el backend: %j es 422 %s', async (query, field) => {
		const error = await client
			.get(`/projects/auditfy/stages/staging/apps/api/logs`, { query })
			.catch((caught: unknown) => caught)
		expect(error).toBeInstanceOf(ApiError)
		expect((error as ApiError).status).toBe(422)
		expect((error as ApiError).message).toMatch(new RegExp(`^${field}: `))
	})

	it('el mock pide el servicio, como el backend (422 service)', async () => {
		const error = await client
			.get(`/projects/auditfy/stages/staging/apps/api/logs`, { query: { tail: 100 } })
			.catch((caught: unknown) => caught)
		expect(error).toBeInstanceOf(ApiError)
		expect((error as ApiError).status).toBe(422)
		expect((error as ApiError).message).toBe('service: es obligatorio')
	})

	it.each([
		['image', 'manda image o images, no los dos', null, 'both'],
		['image', 'una app compose recibe images, por servicio', null, 'compose'],
		['images.worker', 'el servicio no existe en la app', 'worker', 'unknown_service'],
		['images.api', 'no es una referencia de imagen válida', 'api', 'invalid'],
		['images.api', 'algo que el panel no conoce', 'api', 'invalid'],
	] as const)(
		'un 422 de %s ("%s") es ImageRejected con su motivo',
		async (field, reason, service, kind) => {
			server.use(
				http.post(api(`${APP_PATH}/deploys`), () =>
					apiError(422, 'invalid', `${field}: ${reason}`),
				),
			)
			const result = deployUseCases.createDeploy({
				app: APP,
				services: ['api'],
				overrides: { api: 'ghcr.io/x/api:v2' },
			})
			await expect(result).rejects.toBeInstanceOf(ImageRejected)
			await expect(result).rejects.toMatchObject({ service, reason: kind })
		},
	)
})
