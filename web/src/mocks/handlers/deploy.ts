import { delay, http, HttpResponse, type RequestHandler } from 'msw'
// La misma regla que el backend (validate.ImageRef): el mock rechaza lo mismo. Se importa
// el archivo del dominio y no el barrel, que trae la UI.
import { isValidImageRef } from '@/modules/deploy/domain/value-objects/imageRef'
import { api, apiError, invalid } from '../api'
import { deployDb, type DeployView, hashHex, type MockApp, type MockDeploy } from '../db/deploy'
import { requireSession, sessionUser } from '../session'

// Estado, deploys y logs (Fase 2) del backend simulado, con el contrato de
// src/modules/deploy/README.md. Los streams SSE son respuestas `text/event-stream` con un
// ReadableStream (sirven igual al EventSource del navegador y al fetch de node).

const APP = '/projects/:p/stages/:s/apps/:a'

type Params = Record<string, string | readonly string[] | undefined>

function param(params: Params, name: string): string {
	const value = params[name]
	return Array.isArray(value) ? String(value[0] ?? '') : String(value ?? '')
}

function appOf(params: Params): MockApp {
	return deployDb.app(param(params, 'p'), param(params, 's'), param(params, 'a'))
}

function iso(ms: number | null): string | null {
	return ms === null ? null : new Date(ms).toISOString()
}

function deployDto(deploy: MockDeploy, current: DeployView) {
	return {
		id: deploy.id,
		status: current.status,
		images: current.images,
		trigger: deploy.trigger,
		actor_type: deploy.actorType,
		actor_name: deploy.actorName,
		error: current.error,
		created_at: new Date(deploy.createdAt).toISOString(),
		started_at: iso(current.startedAt),
		finished_at: iso(current.finishedAt),
	}
}

function deployNotFound(id: string) {
	return apiError(404, 'not_found', `no existe el deploy #${id}`)
}

function findDeploy(app: MockApp, raw: string): MockDeploy | undefined {
	const id = Number(raw)
	return Number.isInteger(id) ? deployDb.find(app, id) : undefined
}

// --- Estado -----------------------------------------------------------------

function wave(now: number, period: number, phase: number): number {
	return Math.sin(now / period + phase)
}

function statusDto(app: MockApp, now: number) {
	const current = deployDb.current(app, now)
	if (!current) return { services: [] }
	const currentView = deployDb.view(app, current, now)
	const finished = currentView.finishedAt ?? now
	const previous = deployDb.rollbackSource(app, now)
	const failed = [...app.deploys]
		.sort((a, b) => b.id - a.id)
		.find((deploy) => deployDb.view(app, deploy, now).status === 'failed')
	const starting = deployDb.startingTask(app, now)
	const seedPhase = Number.parseInt(hashHex(app.app, 4), 16) / 0xffff

	return {
		services: app.services.map((service, index) => {
			const tasks: Record<string, unknown>[] = []
			for (let slot = 1; slot <= app.replicas; slot += 1) {
				tasks.push({
					id: hashHex(`${current.id}-${service}-${slot}`, 25),
					slot,
					state: 'running',
					desired_state: 'running',
					health: 'healthy',
					node: 'manager-1',
					since: iso(finished - (app.replicas - slot) * 14_000),
					error: null,
				})
			}
			if (starting) {
				tasks.push({
					id: hashHex(`next-${service}-${now >> 16}`, 25),
					slot: 1,
					state: 'starting',
					desired_state: 'running',
					health: 'starting',
					node: 'manager-1',
					since: iso(now - 2000),
					error: null,
				})
			}
			if (previous) {
				tasks.push({
					id: hashHex(`${previous.id}-${service}-1`, 25),
					slot: 1,
					state: 'shutdown',
					desired_state: 'shutdown',
					health: null,
					node: 'manager-1',
					since: iso(finished),
					error: null,
				})
			}
			if (failed) {
				const failedView = deployDb.view(app, failed, now)
				tasks.push({
					id: hashHex(`${failed.id}-${service}-failed`, 25),
					slot: 1,
					state: 'failed',
					desired_state: 'shutdown',
					health: 'unhealthy',
					node: 'manager-1',
					since: iso(failedView.finishedAt),
					error:
						failed.scenario === 'healthcheck' ? 'unhealthy container' : 'no such image',
				})
			}
			const cpu = 3.6 + 1.8 * wave(now, 9000, seedPhase + index) + 0.6 * wave(now, 2300, 1)
			const memory = (298 + 5 * wave(now, 13_000, seedPhase)) * 1024 * 1024 * app.replicas
			return {
				name: service,
				replicas_desired: app.replicas,
				replicas_running: app.replicas,
				cpu_percent: Math.round(Math.max(0.1, cpu) * 10) / 10,
				memory_bytes: Math.round(memory),
				memory_limit_bytes: deployDb.memoryLimit,
				tasks,
			}
		}),
	}
}

// --- Cuerpo de POST …/deploys ------------------------------------------------

type DeployBody = { image?: unknown; images?: unknown }

function currentImages(app: MockApp, now: number): Record<string, string> {
	const current = deployDb.current(app, now) ?? [...app.deploys].sort((a, b) => b.id - a.id)[0]
	if (current) return { ...current.images }
	return Object.fromEntries(
		app.services.map((service) => [service, `ghcr.io/${app.project}/${service}:latest`]),
	)
}

function imagesFromBody(
	app: MockApp,
	body: DeployBody,
	now: number,
): Record<string, string> | Response {
	const unknownKey = Object.keys(body).find((key) => key !== 'image' && key !== 'images')
	if (unknownKey) return apiError(400, 'invalid_json', `campo desconocido "${unknownKey}"`)
	const images = currentImages(app, now)
	if (body.image !== undefined && body.images !== undefined) {
		return invalid('image', 'manda image o images, no los dos')
	}
	if (body.image !== undefined) {
		if (app.services.length !== 1 || app.services[0] !== app.app) {
			return invalid('image', 'una app compose recibe images por servicio')
		}
		if (typeof body.image !== 'string' || !isValidImageRef(body.image)) {
			return invalid('image', 'no es una referencia de imagen válida')
		}
		return { [app.app]: body.image.split('@')[0] ?? body.image }
	}
	if (body.images !== undefined) {
		if (typeof body.images !== 'object' || body.images === null) {
			return invalid('images', 'tiene que ser un objeto servicio → imagen')
		}
		for (const [service, ref] of Object.entries(body.images as Record<string, unknown>)) {
			if (!app.services.includes(service)) {
				return invalid(`images.${service}`, 'el servicio no existe en la app')
			}
			if (typeof ref !== 'string' || !isValidImageRef(ref)) {
				return invalid(`images.${service}`, 'no es una referencia de imagen válida')
			}
			images[service] = ref.split('@')[0] ?? ref
		}
	}
	return images
}

function accepted(params: Params, deploy: MockDeploy) {
	const path = `/api/v1/projects/${param(params, 'p')}/stages/${param(params, 's')}/apps/${param(params, 'a')}/deploys/${deploy.id}`
	return HttpResponse.json({ id: deploy.id, status_url: path }, { status: 202 })
}

// --- SSE ---------------------------------------------------------------------

const encoder = new TextEncoder()

/** Un evento SSE con `event:`, `id:` opcional y el `data:` (una línea por renglón). */
function sseEvent(event: string, data: string, id?: string): Uint8Array {
	const lines = [`event: ${event}`]
	if (id !== undefined) lines.push(`id: ${id}`)
	for (const line of data.split('\n')) lines.push(`data: ${line}`)
	return encoder.encode(`${lines.join('\n')}\n\n`)
}

const SSE_HEADERS = {
	'content-type': 'text/event-stream',
	'cache-control': 'no-cache',
	connection: 'keep-alive',
}

/**
 * Un stream que corre `produce` hasta que termina o el cliente se va (cierra el
 * EventSource o aborta el fetch). `send` devuelve false si ya no hay a quién mandar.
 */
function sseResponse(
	request: Request,
	produce: (send: (chunk: Uint8Array) => boolean, isOpen: () => boolean) => Promise<void>,
): Response {
	let open = true
	const stream = new ReadableStream<Uint8Array>({
		async start(controller) {
			const isOpen = () => open && !request.signal.aborted
			const send = (chunk: Uint8Array): boolean => {
				if (!isOpen()) return false
				try {
					controller.enqueue(chunk)
					return true
				} catch {
					open = false
					return false
				}
			}
			try {
				await produce(send, isOpen)
			} finally {
				if (open) {
					open = false
					try {
						controller.close()
					} catch {
						// El cliente ya lo canceló.
					}
				}
			}
		},
		cancel() {
			open = false
		},
	})
	return new Response(stream, { headers: SSE_HEADERS })
}

/** Cada cuánto el stream del log mira si el deploy avanzó. */
const LOG_TICK_MS = 250

function logStream(request: Request, app: MockApp, deploy: MockDeploy, from: number) {
	return sseResponse(request, async (send, isOpen) => {
		let offset = from
		let status: string | null = null
		while (isOpen()) {
			const current = deployDb.view(app, deploy, Date.now())
			// Las líneas nuevas primero, después el estado: el cierre (`succeeded`) llega
			// cuando el log ya está completo.
			const rest = current.log.slice(offset)
			const lines = rest.split('\n')
			lines.pop()
			for (const line of lines) {
				offset += line.length + 1
				if (!send(sseEvent('log', line, String(offset)))) return
			}
			if (current.status !== status) {
				status = current.status
				if (!send(sseEvent('status', status))) return
			}
			if (current.status !== 'queued' && current.status !== 'running') return
			await delay(LOG_TICK_MS)
		}
	})
}

// Lo que escribe la app de ejemplo, con el nivel como prefijo (el panel lo saca).
const LOG_POOL: readonly string[] = [
	'INFO GET /health 200 · 1 ms',
	'INFO GET /v1/audits?page=1 200 · 41 ms',
	'INFO POST /v1/audits 201 · 148 ms',
	'INFO audit a7f2c encolada → nats audits.created',
	'INFO nats audits.created a7f2c recibido',
	'INFO GET /v1/findings?audit=a7f2c 200 · 22 ms',
	'INFO PATCH /v1/findings/f_91a 200 · 64 ms',
	'WARN consulta lenta · SELECT … FROM findings WHERE audit_id = $1 · 812 ms',
	'INFO GET /v1/me 200 · 9 ms',
	'ERROR redis: ECONNRESET · reintentando en 500 ms',
	'INFO redis: reconectado',
	'INFO POST /v1/exports 202 · 31 ms',
	'INFO export e_33d · 1.240 filas → s3://uploads/exports/e_33d.csv',
	'WARN token expirado · usuario u_8f2a · 401',
	'INFO POST /v1/auth/refresh 200 · 12 ms',
	'{"level":"info","msg":"GET /v1/findings?audit=a7f2c&page=2 200 · 27 ms"}',
	// Con colores de terminal (el panel los saca) y en logfmt.
	'\u001b[33mWARN\u001b[0m cola de exports con 12 pendientes',
	'time=2026-10-10T14:32:07Z level=error msg="no se pudo subir e_33e" retry=2',
]

/** Cada cuánto llega una línea nueva a los logs en vivo. */
const APP_LOG_TICK_MS = 1100
/** Para que un stream olvidado no quede corriendo para siempre en la pestaña. */
const APP_LOG_MAX_MS = 15 * 60_000

function appLogLine(app: MockApp, service: string, sequence: number, time: number): string {
	const slot = (sequence % app.replicas) + 1
	return JSON.stringify({
		time: new Date(time).toISOString(),
		task: `${service}.${slot}`,
		stream: sequence % 11 === 9 ? 'stderr' : 'stdout',
		line: LOG_POOL[sequence % LOG_POOL.length],
	})
}

function appLogStream(
	request: Request,
	app: MockApp,
	service: string,
	tail: number,
	follow: boolean,
) {
	return sseResponse(request, async (send, isOpen) => {
		const now = Date.now()
		let sequence = Math.floor(now / APP_LOG_TICK_MS)
		for (let back = tail; back > 0; back -= 1) {
			const time = now - back * APP_LOG_TICK_MS
			if (!send(sseEvent('log', appLogLine(app, service, sequence - back, time)))) return
		}
		if (!follow) return
		const until = now + APP_LOG_MAX_MS
		while (isOpen() && Date.now() < until) {
			await delay(APP_LOG_TICK_MS)
			sequence += 1
			if (!send(sseEvent('log', appLogLine(app, service, sequence, Date.now())))) return
		}
	})
}

// --- Handlers ------------------------------------------------------------------

/** Lo que tarda en responder una escritura (para ver el botón cargando); escala con los deploys. */
function latency(): number {
	return Math.round(150 * Math.min(1, deployDb.state.liveScale))
}

export const deployHandlers: RequestHandler[] = [
	http.get(api(`${APP}/status`), ({ params }) => {
		const denied = requireSession()
		if (denied) return denied
		return HttpResponse.json(statusDto(appOf(params), Date.now()))
	}),

	http.get(api(`${APP}/deploys`), ({ params }) => {
		const denied = requireSession()
		if (denied) return denied
		const app = appOf(params)
		const now = Date.now()
		const list = [...app.deploys]
			.sort((a, b) => b.id - a.id)
			.map((deploy) => deployDto(deploy, deployDb.view(app, deploy, now)))
		return HttpResponse.json(list)
	}),

	http.post(api(`${APP}/deploys`), async ({ params, request }) => {
		const denied = requireSession()
		if (denied) return denied
		const app = appOf(params)
		const raw = await request.text()
		let body: unknown = {}
		if (raw.trim()) {
			try {
				body = JSON.parse(raw)
			} catch {
				return apiError(400, 'invalid_json', 'el cuerpo no es JSON')
			}
		}
		if (typeof body !== 'object' || body === null || Array.isArray(body)) {
			return apiError(400, 'invalid_json', 'el cuerpo tiene que ser un objeto')
		}
		const now = Date.now()
		const images = imagesFromBody(app, body as DeployBody, now)
		if (images instanceof Response) return images
		const deploy = deployDb.create(
			app,
			{ images, trigger: 'panel', actorName: sessionUser()?.username ?? 'felipe' },
			now,
		)
		await delay(latency())
		return accepted(params, deploy)
	}),

	http.post(api(`${APP}/rollback`), async ({ params }) => {
		const denied = requireSession()
		if (denied) return denied
		const app = appOf(params)
		const now = Date.now()
		const source = deployDb.rollbackSource(app, now)
		await delay(latency())
		if (!source) {
			return apiError(
				409,
				'no_previous_deploy',
				'no hay un deploy exitoso anterior al actual',
			)
		}
		const deploy = deployDb.create(
			app,
			{
				images: { ...source.images },
				trigger: 'rollback',
				actorName: sessionUser()?.username ?? 'felipe',
				rollbackOf: source.id,
				scenario: 'success',
			},
			now,
		)
		return accepted(params, deploy)
	}),

	http.get(api(`${APP}/deploys/:id`), ({ params }) => {
		const denied = requireSession()
		if (denied) return denied
		const app = appOf(params)
		const deploy = findDeploy(app, param(params, 'id'))
		if (!deploy) return deployNotFound(param(params, 'id'))
		return HttpResponse.json(deployDto(deploy, deployDb.view(app, deploy, Date.now())))
	}),

	http.get(api(`${APP}/deploys/:id/log`), ({ params, request }) => {
		const denied = requireSession()
		if (denied) return denied
		const app = appOf(params)
		const deploy = findDeploy(app, param(params, 'id'))
		if (!deploy) return deployNotFound(param(params, 'id'))
		const raw = new URL(request.url).searchParams.get('offset') ?? '0'
		const offset = Number(raw)
		if (!Number.isInteger(offset) || offset < 0) {
			return invalid('offset', 'tiene que ser un entero ≥ 0')
		}
		const current = deployDb.view(app, deploy, Date.now())
		const start = Math.min(offset, current.log.length)
		return HttpResponse.json({
			chunk: current.log.slice(start),
			next_offset: current.log.length,
			status: current.status,
		})
	}),

	http.get(api(`${APP}/deploys/:id/log/stream`), ({ params, request }) => {
		const denied = requireSession()
		if (denied) return denied
		const app = appOf(params)
		const deploy = findDeploy(app, param(params, 'id'))
		if (!deploy) return deployNotFound(param(params, 'id'))
		const url = new URL(request.url)
		const lastEventId = request.headers.get('last-event-id')
		const from = Number(lastEventId ?? url.searchParams.get('offset') ?? '0')
		return logStream(request, app, deploy, Number.isInteger(from) && from > 0 ? from : 0)
	}),

	http.get(api(`${APP}/logs`), ({ params, request }) => {
		const denied = requireSession()
		if (denied) return denied
		const app = appOf(params)
		const url = new URL(request.url)
		const service = url.searchParams.get('service') ?? app.services[0] ?? ''
		if (!app.services.includes(service)) {
			return apiError(404, 'not_found', `no existe el servicio "${service}"`)
		}
		const tail = Number(url.searchParams.get('tail') ?? '200')
		if (!Number.isInteger(tail) || tail < 0 || tail > 5000) {
			return invalid('tail', 'tiene que ser un entero entre 0 y 5000')
		}
		const follow = url.searchParams.get('follow') === '1'
		return appLogStream(request, app, service, tail, follow)
	}),
]
