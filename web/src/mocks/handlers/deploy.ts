import { delay, http, HttpResponse, type RequestHandler } from 'msw'
// La misma regla que el backend (validate.ImageRef): el mock rechaza lo mismo. Se importa
// el archivo del dominio y no el barrel, que trae la UI.
import { isValidImageRef } from '@/modules/deploy/domain/value-objects/imageRef'
import { api, apiError, invalid } from '../api'
import {
	deployDb,
	type DeployView,
	hashHex,
	type MockApp,
	type MockDeploy,
	resolvedOf,
} from '../db/deploy'
import { ingressDb } from '../db/ingress'
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
		actor_id: deploy.actorType === 'user' ? 1 : 4,
		actor_name: deploy.actorName,
		error: current.error,
		created_at: new Date(deploy.createdAt).toISOString(),
		started_at: iso(current.startedAt),
		finished_at: iso(current.finishedAt),
		superseded_by: current.status === 'superseded' ? (deploy.supersededBy ?? null) : null,
		requested_images: current.requestedImages,
		routes_warning: current.routesWarning,
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
		// Lo que corre en Swarm: los servicios del deploy actual.
		services: deployDb.swarmServices(app, now).map((service, index) => {
			const tasks: Record<string, unknown>[] = []
			for (let slot = 1; slot <= app.replicas; slot += 1) {
				// La réplica 2 corre en otro nodo: el backend no la mide (solo el nodo de muelle).
				const local = slot !== 2
				const cpu =
					1.8 +
					0.9 * wave(now, 9000, seedPhase + index + slot) +
					0.3 * wave(now, 2300, slot)
				const memory = (298 + 5 * wave(now, 13_000, seedPhase + slot)) * 1024 * 1024
				tasks.push({
					id: hashHex(`${current.id}-${service}-${slot}`, 25),
					slot,
					state: 'running',
					desired_state: 'running',
					// El health sale del contenedor en el nodo de muelle: el de otro nodo, null.
					health: local ? 'healthy' : null,
					node: local ? 'manager-1' : 'worker-1',
					since: iso(finished - (app.replicas - slot) * 14_000),
					error: null,
					cpu_percent: local ? Math.round(Math.max(0.1, cpu) * 10) / 10 : null,
					memory_bytes: local ? Math.round(memory) : null,
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
			// Primero las vigentes; después por slot y de la más nueva a la más vieja.
			tasks.sort(
				(a, b) =>
					Number(b.desired_state === 'running') - Number(a.desired_state === 'running') ||
					Number(a.slot ?? 0) - Number(b.slot ?? 0) ||
					String(b.since ?? '').localeCompare(String(a.since ?? '')),
			)
			const measured = tasks.filter((task) => typeof task.cpu_percent === 'number')
			const sum = (key: 'cpu_percent' | 'memory_bytes') =>
				measured.length === 0
					? null
					: measured.reduce((total, task) => total + Number(task[key] ?? 0), 0)
			return {
				name: service,
				image: currentView.images[service] ?? null,
				replicas_desired: app.replicas,
				replicas_running: app.replicas,
				// Las sumas son para otros clientes: el panel lee las de cada tarea.
				cpu_percent:
					sum('cpu_percent') === null
						? null
						: Math.round(Number(sum('cpu_percent')) * 10) / 10,
				memory_bytes: sum('memory_bytes'),
				memory_limit_bytes: deployDb.memoryLimit,
				update_state: starting ? 'updating' : 'completed',
				tasks,
			}
		}),
	}
}

// --- Cuerpo de POST …/deploys ------------------------------------------------

type DeployBody = { image?: unknown; images?: unknown }

/**
 * Las imágenes del deploy nuevo (contrato §2.2): el override de cada servicio que lo trae
 * y, para el resto, `baseImages`. Valida contra la configuración de ahora (el catálogo):
 * `image` solo en una app de formulario, e `images.<servicio>` con un servicio que existe.
 */
function imagesFromBody(
	app: MockApp,
	body: DeployBody,
	now: number,
): Record<string, string> | Response {
	const unknownKey = Object.keys(body).find((key) => key !== 'image' && key !== 'images')
	if (unknownKey) return apiError(400, 'invalid_json', `campo desconocido "${unknownKey}"`)
	if (body.image !== undefined && body.images !== undefined) {
		return invalid('image', 'manda image o images, no los dos')
	}
	const config = deployDb.config(app)
	const images = deployDb.baseImages(app, now)
	if (body.image !== undefined) {
		if (config.kind !== 'service') {
			return invalid('image', 'una app compose recibe images, por servicio')
		}
		if (typeof body.image !== 'string' || !isValidImageRef(body.image)) {
			return invalid('image', 'no es una referencia de imagen válida')
		}
		return { ...images, [app.app]: body.image }
	}
	if (body.images !== undefined) {
		if (typeof body.images !== 'object' || body.images === null) {
			return invalid('images', 'tiene que ser un objeto servicio → imagen')
		}
		for (const [service, ref] of Object.entries(body.images as Record<string, unknown>)) {
			if (!(service in config.images)) {
				return invalid(`images.${service}`, 'el servicio no existe en la app')
			}
			if (typeof ref !== 'string' || !isValidImageRef(ref)) {
				return invalid(`images.${service}`, 'no es una referencia de imagen válida')
			}
			images[service] = ref
		}
	}
	return images
}

function accepted(params: Params, deploy: MockDeploy) {
	const path = `/api/v1/projects/${param(params, 'p')}/stages/${param(params, 's')}/apps/${param(params, 'a')}/deploys/${deploy.id}`
	return HttpResponse.json(
		{ id: deploy.id, status: 'queued', status_url: path },
		{ status: 202, headers: { location: path } },
	)
}

/** Lo que pidió el cuerpo: `{image}` es el único servicio de una app de formulario. */
function overridesOf(app: MockApp, body: DeployBody): Record<string, string> {
	if (typeof body.image === 'string') return { [app.app]: body.image }
	if (typeof body.images === 'object' && body.images !== null) {
		return { ...(body.images as Record<string, string>) }
	}
	return {}
}

// --- SSE ---------------------------------------------------------------------

const encoder = new TextEncoder()
const decoder = new TextDecoder()

/** Lo más que trae un `chunk` del polling (contrato §2.5), siempre en líneas completas. */
const LOG_CHUNK_MAX = 64 * 1024
const NEWLINE = 0x0a

/** Desde `offset` (bytes), hasta 64 KiB y cortado en el último `\n`. */
function logChunk(bytes: Uint8Array, offset: number): { chunk: string; next: number } {
	let end = Math.min(bytes.length, offset + LOG_CHUNK_MAX)
	const lastNewline = bytes.lastIndexOf(NEWLINE, end - 1)
	end = lastNewline >= offset ? lastNewline + 1 : offset
	return { chunk: decoder.decode(bytes.subarray(offset, end)), next: end }
}

/** Un evento SSE con `event:`, `id:` opcional y el `data:` (una línea por renglón). */
function sseEvent(event: string, data: string, id?: string): Uint8Array {
	const lines = [`event: ${event}`]
	if (id !== undefined) lines.push(`id: ${id}`)
	for (const line of data.split('\n')) lines.push(`data: ${line}`)
	return encoder.encode(`${lines.join('\n')}\n\n`)
}

/** El backend manda un comentario `: ping` cada ~15 s en los dos streams SSE. */
const SSE_HEARTBEAT_MS = 15_000

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

function isFinal(status: string): boolean {
	return status !== 'queued' && status !== 'running'
}

// Contrato §2.6: el `status` va al conectar solo si el deploy no terminó, y cada vez que
// cambia; el final, siempre después de la última línea. Los `id` son bytes del log.
function logStream(request: Request, app: MockApp, deploy: MockDeploy, from: number) {
	return sseResponse(request, async (send, isOpen) => {
		let offset = from
		let status: string | null = null
		let lastSent = Date.now()
		const first = deployDb.view(app, deploy, Date.now())
		if (!isFinal(first.status)) {
			status = first.status
			if (!send(sseEvent('status', status))) return
		}
		while (isOpen()) {
			const current = deployDb.view(app, deploy, Date.now())
			const bytes = encoder.encode(current.log)
			const rest = decoder.decode(bytes.subarray(offset))
			const lines = rest.split('\n')
			lines.pop()
			for (const line of lines) {
				offset += encoder.encode(line).length + 1
				if (!send(sseEvent('log', line, String(offset)))) return
				lastSent = Date.now()
			}
			if (current.status !== status) {
				status = current.status
				if (!send(sseEvent('status', status))) return
				lastSent = Date.now()
			}
			if (isFinal(current.status)) return
			if (Date.now() - lastSent >= SSE_HEARTBEAT_MS) {
				if (!send(encoder.encode(': ping\n\n'))) return
				lastSent = Date.now()
			}
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
/**
 * La hora de vida de un stream de logs (el backend: 1 h). Al llegar manda `end` con
 * `timeout` y cierra; el panel reconecta enseguida.
 */
const APP_LOG_MAX_MS = 15 * 60_000

/** El máximo de `tail` (y de lo que se manda de una vez con `since`). */
const MAX_LOG_TAIL = 5000

/** La línea `sequence` se escribió en `sequence * APP_LOG_TICK_MS`: es la misma en cada stream. */
function appLogLine(app: MockApp, service: string, sequence: number): string {
	const slot = (sequence % app.replicas) + 1
	return JSON.stringify({
		time: new Date(sequence * APP_LOG_TICK_MS).toISOString(),
		task: `${service}.${slot}`,
		stream: sequence % 11 === 9 ? 'stderr' : 'stdout',
		line: LOG_POOL[sequence % LOG_POOL.length],
	})
}

/** Desde dónde: las últimas `tail` líneas, o todo lo escrito desde `since` (inclusivo, como Docker). */
/** Las últimas `tail` o, con `since`, todo lo escrito desde ahí (inclusivo), hasta 5000. */
interface LogStart {
	readonly tail: number
	readonly since: number | null
}

function appLogStream(
	request: Request,
	app: MockApp,
	service: string,
	start: LogStart,
	follow: boolean,
) {
	return sseResponse(request, async (send, isOpen) => {
		const now = Date.now()
		const last = Math.floor(now / APP_LOG_TICK_MS)
		const first =
			start.since === null
				? last - start.tail + 1
				: Math.max(Math.ceil(start.since / APP_LOG_TICK_MS), last - MAX_LOG_TAIL + 1)
		for (let sequence = first; sequence <= last; sequence += 1) {
			if (!send(sseEvent('log', appLogLine(app, service, sequence)))) return
		}
		// Sin `follow`: las últimas `tail`, un `end` y se cierra.
		if (!follow) {
			send(sseEvent('end', 'eof'))
			return
		}
		const until = now + APP_LOG_MAX_MS
		let sequence = last
		let heartbeat = now
		while (isOpen()) {
			if (Date.now() >= until) {
				send(sseEvent('end', 'timeout'))
				return
			}
			sequence += 1
			await delay(Math.max(0, sequence * APP_LOG_TICK_MS - Date.now()))
			if (!send(sseEvent('log', appLogLine(app, service, sequence)))) return
			// El comentario de cada ~15 s del backend (las líneas ya mantienen viva la conexión).
			if (Date.now() - heartbeat >= SSE_HEARTBEAT_MS) {
				heartbeat = Date.now()
				if (!send(encoder.encode(': ping\n\n'))) return
			}
		}
	})
}

/** Un nombre de servicio: un slug de hasta 16 caracteres (§5.2 regla 1 del diseño). */
const SERVICE_SLUG = /^[a-z]([a-z0-9-]{0,14}[a-z0-9])?$/

// --- Handlers ------------------------------------------------------------------

/** Lo que tarda en responder una escritura (para ver el botón cargando); escala con los deploys. */
function latency(): number {
	return Math.round(150 * Math.min(1, deployDb.state.liveScale))
}

/** `SyncRoutes` falla si Caddy no acepta la configuración (el paso 7, contrato §2.11). */
function caddyRejects(now: number): boolean {
	return ingressDb.proxy(now).status === 'out_of_sync'
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
			{
				images,
				overrides: overridesOf(app, body as DeployBody),
				trigger: 'panel',
				actorName: sessionUser()?.username ?? 'felipe',
				caddyRejects: caddyRejects(now),
			},
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
		// Un rollback pide las refs resueltas del succeeded anterior, con digest (§2.7): son
		// sus overrides y quedan fijas.
		const requested = resolvedOf(source)
		const deploy = deployDb.create(
			app,
			{
				images: requested,
				overrides: { ...requested },
				trigger: 'rollback',
				actorName: sessionUser()?.username ?? 'felipe',
				rollbackOf: source.id,
				scenario: 'success',
				caddyRejects: caddyRejects(now),
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
		const bytes = encoder.encode(current.log)
		if (offset > bytes.length) return invalid('offset', 'pasa el largo del log')
		const { chunk, next } = logChunk(bytes, offset)
		return HttpResponse.json({
			chunk,
			next_offset: next,
			status: current.status,
			superseded_by: current.status === 'superseded' ? (deploy.supersededBy ?? null) : null,
		})
	}),

	http.get(api(`${APP}/deploys/:id/log/stream`), ({ params, request }) => {
		const denied = requireSession()
		if (denied) return denied
		const app = appOf(params)
		const deploy = findDeploy(app, param(params, 'id'))
		if (!deploy) return deployNotFound(param(params, 'id'))
		const url = new URL(request.url)
		// `Last-Event-ID` gana sobre `?offset`; los dos, como en el polling: un entero ≥ 0 que
		// no pase el largo del log, o `422 offset` antes de abrir el stream.
		const lastEventId = request.headers.get('last-event-id')
		const from = Number(lastEventId ?? url.searchParams.get('offset') ?? '0')
		if (!Number.isInteger(from) || from < 0) {
			return invalid('offset', 'tiene que ser un entero ≥ 0')
		}
		const length = encoder.encode(deployDb.view(app, deploy, Date.now()).log).length
		if (from > length) return invalid('offset', 'pasa el largo del log')
		return logStream(request, app, deploy, from)
	}),

	http.get(api(`${APP}/logs`), ({ params, request }) => {
		const denied = requireSession()
		if (denied) return denied
		const app = appOf(params)
		const url = new URL(request.url)
		// El orden del backend (contrato §2.9): la query, en el orden de la tabla, y recién
		// después el servicio en Swarm.
		const service = url.searchParams.get('service') ?? ''
		if (!service) return invalid('service', 'es obligatorio')
		if (!SERVICE_SLUG.test(service)) {
			return invalid('service', 'tiene que ser un slug de hasta 16 caracteres')
		}
		// Al reconectar el panel manda `since` (la hora de la última línea) en vez de `tail`:
		// con `since`, `tail` no se mira.
		const sinceParam = url.searchParams.get('since')
		const since = sinceParam === null ? null : Date.parse(sinceParam)
		const tail = Number(url.searchParams.get('tail') ?? '200')
		if (since === null && (!Number.isInteger(tail) || tail < 0 || tail > MAX_LOG_TAIL)) {
			return invalid('tail', `tiene que ser un entero entre 0 y ${MAX_LOG_TAIL}`)
		}
		if (since !== null && Number.isNaN(since)) {
			return invalid('since', 'tiene que ser una fecha RFC 3339')
		}
		// Vacío o `0`: las líneas, un `end` y se cierra.
		const followParam = url.searchParams.get('follow') ?? ''
		if (!['', '0', '1'].includes(followParam)) return invalid('follow', 'tiene que ser 0 o 1')
		const follow = followParam === '1'
		if (!deployDb.swarmServices(app, Date.now()).includes(service)) {
			return apiError(404, 'not_found', `no existe el servicio "${service}"`)
		}
		return since === null
			? appLogStream(request, app, service, { tail, since: null }, follow)
			: appLogStream(request, app, service, { tail: MAX_LOG_TAIL, since }, follow)
	}),
]
