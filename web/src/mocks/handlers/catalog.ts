import { http, HttpResponse, type RequestHandler } from 'msw'
import { api, apiError, invalid, noContent, notFound } from '../api'
import { catalogDb, type MockApp, type MockHealthcheck, type MockSpec } from '../db/catalog'
import { ingressDb } from '../db/ingress'
import { requireSession } from '../session'

// Proyectos, stages y apps (catalog/infra/in/http/handler.go). Replica las reglas que el
// panel tiene que manejar: slugs, nombres, spec de formulario, compose, `not_empty`,
// `conflict` y `kind_mismatch`. Los mensajes de los 422 son los del backend. Del compose
// (contrato §2.12 y §6.3 del diseño) solo dos de las claves rechazadas, `ports` y `build`,
// y los nombres de servicio repetidos en el stage: alcanza para ver cómo los muestra el
// panel. Una app con dominios (los de `db/ingress.ts`) no se borra: `409 not_empty`.

const SLUG_16 = /^[a-z]([a-z0-9-]{0,14}[a-z0-9])?$/
const SLUG_12 = /^[a-z]([a-z0-9-]{0,10}[a-z0-9])?$/
const HEALTH_PATH = /^\/[A-Za-z0-9._~/?=%+-]*$/
const MOUNT = /^\/[A-Za-z0-9._/-]{1,255}$/
const IMAGE =
	/^(?:[a-zA-Z0-9.-]+(?::[0-9]+)?\/)?[a-z0-9]+(?:[._-][a-z0-9]+)*(?:\/[a-z0-9]+(?:[._-][a-z0-9]+)*)*(?::[\w][\w.-]{0,127})?(?:@sha256:[a-f0-9]{64})?$/

const slugMessage = (slug: string, max: number) =>
	`"${slug}" no es válido: hasta ${max} caracteres, solo a-z, 0-9 y '-', empieza con letra y no termina en '-'`

class Invalid extends Error {
	constructor(
		readonly field: string,
		readonly reason: string,
	) {
		super(`${field}: ${reason}`)
	}
}

type Body = Record<string, unknown>

async function readBody(request: Request, allowed: readonly string[]): Promise<Body | Response> {
	let body: unknown
	try {
		body = await request.json()
	} catch {
		return apiError(400, 'invalid_json', 'JSON inválido')
	}
	if (!body || typeof body !== 'object' || Array.isArray(body)) {
		return apiError(400, 'invalid_json', 'JSON inválido: el cuerpo debe ser un objeto')
	}
	const unknown = Object.keys(body).find((key) => !allowed.includes(key))
	if (unknown) {
		return apiError(400, 'invalid_json', `JSON inválido: el campo "${unknown}" no existe`)
	}
	return body as Body
}

function checkName(name: unknown): string {
	if (typeof name !== 'string' || name.trim() === '') throw new Invalid('name', 'es obligatorio')
	if ([...name].length > 64) throw new Invalid('name', 'no puede superar 64 caracteres')
	return name
}

function checkTimeout(value: unknown): number {
	if (typeof value !== 'number' || !Number.isInteger(value) || value < 30 || value > 3600) {
		throw new Invalid('deploy_timeout_s', 'debe estar entre 30 y 3600')
	}
	return value
}

function checkCompose(value: unknown): string {
	if (typeof value !== 'string' || value.trim() === '') {
		throw new Invalid('compose_yaml', 'es obligatorio')
	}
	if (new TextEncoder().encode(value).length > 256 * 1024) {
		throw new Invalid('compose_yaml', 'no puede superar 256 KiB')
	}
	return value
}

function intIn(value: unknown, min: number, max: number): value is number {
	return typeof value === 'number' && Number.isInteger(value) && value >= min && value <= max
}

const SPEC_KEYS = ['image', 'port', 'replicas', 'command', 'healthcheck', 'volumes', 'resources']
const HEALTH_KEYS = [
	'type',
	'path',
	'command',
	'interval_s',
	'timeout_s',
	'retries',
	'start_period_s',
]

function rejectUnknown(value: object, allowed: readonly string[], prefix: string): void {
	const unknown = Object.keys(value).find((key) => !allowed.includes(key))
	if (unknown) {
		throw new Invalid('spec', `JSON inválido: el campo "${prefix}${unknown}" no existe`)
	}
}

function checkHealthcheck(raw: unknown): MockHealthcheck {
	if (raw === undefined || raw === null) return { type: 'none' }
	if (typeof raw !== 'object')
		throw new Invalid('spec', 'JSON inválido: no tiene la forma esperada')
	rejectUnknown(raw, HEALTH_KEYS, 'healthcheck.')
	const hc = raw as MockHealthcheck
	if (hc.type === 'none') {
		if (hc.path || hc.command?.length) {
			throw new Invalid('healthcheck', 'con type none no lleva path ni command')
		}
		if (hc.interval_s || hc.timeout_s || hc.retries || hc.start_period_s) {
			throw new Invalid(
				'healthcheck',
				'con type none no lleva intervalos, timeouts ni reintentos',
			)
		}
		return { type: 'none' }
	}
	if (hc.type === 'http' || hc.type === 'bun-http') {
		if (typeof hc.path !== 'string' || !HEALTH_PATH.test(hc.path) || hc.path.length > 256) {
			throw new Invalid(
				'healthcheck.path',
				'debe empezar con / y usar solo letras, números y ._~/?=%+-',
			)
		}
		if (hc.command?.length)
			throw new Invalid('healthcheck.command', 'solo aplica a type command')
	} else if (hc.type === 'command') {
		if (!Array.isArray(hc.command) || hc.command.length === 0 || hc.command.length > 64) {
			throw new Invalid(
				'healthcheck.command',
				'es obligatorio con type command (máximo 64 argumentos)',
			)
		}
		if (hc.path) throw new Invalid('healthcheck.path', 'solo aplica a type http o bun-http')
	} else {
		throw new Invalid('healthcheck.type', 'debe ser http, bun-http, command o none')
	}
	const timings = {
		interval_s: hc.interval_s || 10,
		timeout_s: hc.timeout_s || 5,
		retries: hc.retries || 3,
		start_period_s: hc.start_period_s || 20,
	}
	if (!intIn(timings.interval_s, 1, 3600)) {
		throw new Invalid('healthcheck.interval_s', 'debe estar entre 1 y 3600')
	}
	if (!intIn(timings.timeout_s, 1, 600)) {
		throw new Invalid('healthcheck.timeout_s', 'debe estar entre 1 y 600')
	}
	if (!intIn(timings.retries, 1, 20))
		throw new Invalid('healthcheck.retries', 'debe estar entre 1 y 20')
	if (!intIn(timings.start_period_s, 1, 3600)) {
		throw new Invalid('healthcheck.start_period_s', 'debe estar entre 1 y 3600')
	}
	return hc.type === 'command'
		? { type: 'command', command: hc.command ?? [], ...timings }
		: { type: hc.type, path: hc.path ?? '', ...timings }
}

/** `vo.ParseAppSpec`: defaults, validación y el spec normalizado que se guarda. */
function checkSpec(raw: unknown): MockSpec {
	if (raw === undefined || raw === null) throw new Invalid('spec', 'es obligatorio')
	if (typeof raw !== 'object' || Array.isArray(raw)) {
		throw new Invalid('spec', 'JSON inválido: no tiene la forma esperada')
	}
	rejectUnknown(raw, SPEC_KEYS, '')
	const spec = raw as Partial<MockSpec> & { replicas?: number | null }
	const image = spec.image ?? ''
	if (typeof image !== 'string' || image === '' || image.length > 512) {
		throw new Invalid('image', 'es obligatoria y de hasta 512 caracteres')
	}
	if (!IMAGE.test(image)) {
		throw new Invalid(
			'image',
			'la imagen no es una referencia válida de Docker (registro/repositorio[:tag][@sha256:…], en minúsculas)',
		)
	}
	if (!intIn(spec.port, 1, 65535)) throw new Invalid('port', 'debe estar entre 1 y 65535')
	const replicas = spec.replicas ?? 1
	if (!intIn(replicas, 0, 50)) throw new Invalid('replicas', 'debe estar entre 0 y 50')
	const command = spec.command ?? []
	if (command.length > 64) throw new Invalid('command', 'máximo 64 argumentos')
	const healthcheck = checkHealthcheck(spec.healthcheck)

	const volumes = spec.volumes ?? []
	if (volumes.length > 10) throw new Invalid('volumes', 'máximo 10')
	const names = new Set<string>()
	const mounts = new Set<string>()
	volumes.forEach((volume, index) => {
		const field = `volumes[${index}]`
		if (!SLUG_16.test(volume.name))
			throw new Invalid(`${field}.name`, slugMessage(volume.name, 16))
		const clean = `/${volume.mount
			.split('/')
			.filter((part) => part && part !== '.')
			.join('/')}`
		if (!MOUNT.test(volume.mount) || clean !== volume.mount || volume.mount.includes('/..')) {
			throw new Invalid(
				`${field}.mount`,
				'debe ser una ruta absoluta limpia distinta de /, de hasta 256 caracteres, con solo letras, números y ._/-',
			)
		}
		if (names.has(volume.name))
			throw new Invalid(`${field}.name`, `"${volume.name}" está repetido`)
		if (mounts.has(volume.mount)) {
			throw new Invalid(`${field}.mount`, `"${volume.mount}" está repetido`)
		}
		names.add(volume.name)
		mounts.add(volume.mount)
	})

	const resources = spec.resources ?? null
	if (resources) {
		const { memory_mb: memory, cpus } = resources
		if (memory && !intIn(memory, 16, 1 << 20)) {
			throw new Invalid('resources.memory_mb', 'debe estar entre 16 y 1048576')
		}
		if (cpus && !(cpus >= 0.01 && cpus <= 256)) {
			throw new Invalid('resources.cpus', 'debe estar entre 0.01 y 256')
		}
	}
	return {
		image,
		port: spec.port,
		replicas,
		command,
		healthcheck,
		volumes: volumes.map((volume) => ({ name: volume.name, mount: volume.mount })),
		resources: resources
			? {
					...(resources.memory_mb ? { memory_mb: resources.memory_mb } : {}),
					...(resources.cpus ? { cpus: resources.cpus } : {}),
				}
			: null,
	}
}

/** Un puerto de `expose`: un entero o `N/tcp`; los rangos y los `${VAR}` no cuentan. */
function exposedPort(value: string): number | null {
	const match = /^(\d+)(\/tcp)?$/.exec(value.trim().replace(/^["']|["']$/g, ''))
	const port = match ? Number(match[1]) : Number.NaN
	return Number.isInteger(port) && port >= 1 && port <= 65535 ? port : null
}

/**
 * Los servicios de un compose y los puertos de su `expose`, como `compose.Services` del
 * backend (contrato §2.12): por nombre, con los puertos ordenados. Un parser de líneas
 * alcanza para los compose del mock (`expose: [..]` o una lista con `- `).
 */
function composeServices(yaml: string): { name: string; ports: number[] }[] {
	const services = new Map<string, Set<number>>()
	let inServices = false
	let serviceIndent = -1
	let current: Set<number> | null = null
	let exposeIndent = -1
	const add = (value: string) => {
		const port = exposedPort(value)
		if (port !== null) current?.add(port)
	}
	for (const raw of yaml.split('\n')) {
		const text = raw.trim()
		if (text === '' || text.startsWith('#')) continue
		const indent = raw.length - raw.trimStart().length
		if (indent === 0) {
			inServices = /^services:\s*$/.test(text)
			serviceIndent = -1
			current = null
			continue
		}
		if (!inServices) continue
		if (serviceIndent === -1) serviceIndent = indent
		if (indent === serviceIndent) {
			current = new Set()
			services.set(text.replace(/:.*$/, ''), current)
			exposeIndent = -1
			continue
		}
		if (exposeIndent !== -1 && indent > exposeIndent && text.startsWith('- ')) {
			add(text.slice(2))
			continue
		}
		exposeIndent = -1
		const expose = /^expose:\s*(.*)$/.exec(text)
		if (!expose) continue
		const inline = expose[1] ?? ''
		if (inline.startsWith('[')) inline.slice(1, -1).split(',').forEach(add)
		else exposeIndent = indent
	}
	return [...services]
		.map(([name, ports]) => ({ name, ports: [...ports].sort((a, b) => a - b) }))
		.sort((a, b) => a.name.localeCompare(b.name))
}

/** Los motivos del backend para las dos claves de servicio que el mock rechaza. */
const REJECTED_KEYS: Readonly<Record<string, string>> = {
	ports: 'publicar puertos no está soportado en la v1: los dominios van por Caddy',
	build: 'construir imágenes no está soportado: las imágenes vienen de un registry',
}

/** La primera clave rechazada de un servicio (`services.web.ports: …`), o null. */
function rejectedKey(yaml: string): string | null {
	let inServices = false
	let serviceIndent = -1
	let keyIndent = -1
	let service = ''
	for (const raw of yaml.split('\n')) {
		const text = raw.trim()
		if (text === '' || text.startsWith('#')) continue
		const indent = raw.length - raw.trimStart().length
		if (indent === 0) {
			inServices = /^services:\s*$/.test(text)
			serviceIndent = -1
			continue
		}
		if (!inServices) continue
		if (serviceIndent === -1) serviceIndent = indent
		if (indent === serviceIndent) {
			service = text.replace(/:.*$/, '')
			keyIndent = -1
			continue
		}
		if (keyIndent === -1) keyIndent = indent
		const key = indent === keyIndent ? /^([a-z_]+):/.exec(text)?.[1] : undefined
		const reason = key ? REJECTED_KEYS[key] : undefined
		if (key && reason) return `services.${service}.${key}: ${reason}`
	}
	return null
}

/** Los nombres de servicio de cada otra app del stage (§5.2 regla 2: comparten la red). */
function stageServices(project: string, stage: string, except: string): Map<string, MockApp> {
	const names = new Map<string, MockApp>()
	for (const other of catalogDb.appsOf(project, stage)) {
		if (other.slug === except) continue
		for (const service of servicesOf(other)) names.set(service.name, other)
	}
	return names
}

/** Lo que valida catalog al guardar un compose, además del tamaño. */
function checkComposeRules(project: string, stage: string, slug: string, yaml: string): void {
	const rejected = rejectedKey(yaml)
	if (rejected) throw new Invalid('compose_yaml', rejected)
	const taken = stageServices(project, stage, slug)
	for (const service of composeServices(yaml)) {
		const owner = taken.get(service.name)
		if (owner) {
			throw new Invalid(
				'compose_yaml',
				`el servicio "${service.name}" ya existe en la app "${owner.slug}" del stage: los servicios de un stage comparten la red`,
			)
		}
	}
}

/** Una app de formulario nueva no puede llamarse como un servicio de una compose del stage. */
function checkServiceSlug(project: string, stage: string, slug: string): void {
	const owner = stageServices(project, stage, slug).get(slug)
	if (owner?.kind === 'compose') {
		throw new Invalid(
			'slug',
			`"${slug}" ya es un servicio de la app compose "${owner.slug}" del stage`,
		)
	}
}

/** `services` del `AppOutput` (contrato §2.12): de la configuración guardada, no de Swarm. */
function servicesOf(app: MockApp): { name: string; ports: number[] }[] {
	if (app.kind === 'compose') return composeServices(app.compose_yaml ?? '')
	const port = app.spec?.port ?? 0
	return [{ name: app.slug, ports: port > 0 ? [port] : [] }]
}

/** El `AppOutput` del backend: `spec` u `compose_yaml` según el tipo, y `pending_changes`. */
function appOutput(app: MockApp) {
	const pending =
		app.last_deployed_at !== null &&
		new Date(app.config_changed_at).getTime() > new Date(app.last_deployed_at).getTime()
	return {
		project: app.project,
		stage: app.stage,
		slug: app.slug,
		kind: app.kind,
		...(app.kind === 'service' ? { spec: app.spec } : { compose_yaml: app.compose_yaml }),
		deploy_timeout_s: app.deploy_timeout_s,
		config_changed_at: app.config_changed_at,
		last_deployed_at: app.last_deployed_at,
		pending_changes: pending,
		created_at: app.created_at,
		services: servicesOf(app),
	}
}

function failed(error: unknown): Response {
	if (error instanceof Invalid) return invalid(error.field, error.reason)
	throw error
}

const projects = api('/projects')
const project = api('/projects/:p')
const stages = api('/projects/:p/stages')
const stage = api('/projects/:p/stages/:s')
const apps = api('/projects/:p/stages/:s/apps')
const app = api('/projects/:p/stages/:s/apps/:a')

type Params = { p: string; s: string; a: string }

export const catalogHandlers: RequestHandler[] = [
	http.get(projects, () => requireSession() ?? HttpResponse.json(catalogDb.state.projects)),

	http.post(projects, async ({ request }) => {
		const denied = requireSession()
		if (denied) return denied
		const body = await readBody(request, ['slug', 'name'])
		if (body instanceof Response) return body
		try {
			const slug = String(body.slug ?? '')
			if (!SLUG_16.test(slug)) throw new Invalid('slug', slugMessage(slug, 16))
			if (slug === 'muelle' || slug === 'docker')
				throw new Invalid('slug', `"${slug}" está reservado`)
			const name = checkName(body.name)
			if (catalogDb.findProject(slug)) return apiError(409, 'conflict', 'ya existe')
			const created = { slug, name, created_at: new Date().toISOString() }
			catalogDb.state.projects.push(created)
			catalogDb.save()
			return HttpResponse.json(created, { status: 201 })
		} catch (error) {
			return failed(error)
		}
	}),

	http.get<Params>(project, ({ params }) => {
		const denied = requireSession()
		if (denied) return denied
		const found = catalogDb.findProject(params.p)
		return found ? HttpResponse.json(found) : notFound()
	}),

	http.patch<Params>(project, async ({ params, request }) => {
		const denied = requireSession()
		if (denied) return denied
		const body = await readBody(request, ['name'])
		if (body instanceof Response) return body
		const found = catalogDb.findProject(params.p)
		if (!found) return notFound()
		try {
			found.name = checkName(body.name)
			catalogDb.save()
			return HttpResponse.json(found)
		} catch (error) {
			return failed(error)
		}
	}),

	http.delete<Params>(project, ({ params }) => {
		const denied = requireSession()
		if (denied) return denied
		if (!catalogDb.findProject(params.p)) return notFound()
		if (catalogDb.stagesOf(params.p).length > 0) {
			return apiError(409, 'not_empty', 'primero hay que borrar lo que contiene')
		}
		catalogDb.state.projects = catalogDb.state.projects.filter((item) => item.slug !== params.p)
		catalogDb.save()
		return noContent()
	}),

	http.get<Params>(stages, ({ params }) => {
		const denied = requireSession()
		if (denied) return denied
		if (!catalogDb.findProject(params.p)) return notFound()
		return HttpResponse.json(catalogDb.stagesOf(params.p))
	}),

	http.post<Params>(stages, async ({ params, request }) => {
		const denied = requireSession()
		if (denied) return denied
		const body = await readBody(request, ['slug'])
		if (body instanceof Response) return body
		try {
			const slug = String(body.slug ?? '')
			if (!SLUG_12.test(slug)) throw new Invalid('slug', slugMessage(slug, 12))
			if (!catalogDb.findProject(params.p)) return notFound()
			if (catalogDb.findStage(params.p, slug)) return apiError(409, 'conflict', 'ya existe')
			const created = { project: params.p, slug, created_at: new Date().toISOString() }
			catalogDb.state.stages.push(created)
			catalogDb.save()
			return HttpResponse.json(created, { status: 201 })
		} catch (error) {
			return failed(error)
		}
	}),

	http.get<Params>(stage, ({ params }) => {
		const denied = requireSession()
		if (denied) return denied
		const found = catalogDb.findStage(params.p, params.s)
		return found ? HttpResponse.json(found) : notFound()
	}),

	http.delete<Params>(stage, ({ params }) => {
		const denied = requireSession()
		if (denied) return denied
		if (!catalogDb.findStage(params.p, params.s)) return notFound()
		if (catalogDb.appsOf(params.p, params.s).length > 0) {
			return apiError(409, 'not_empty', 'primero hay que borrar lo que contiene')
		}
		catalogDb.state.stages = catalogDb.state.stages.filter(
			(item) => !(item.project === params.p && item.slug === params.s),
		)
		catalogDb.save()
		return noContent()
	}),

	http.get<Params>(apps, ({ params }) => {
		const denied = requireSession()
		if (denied) return denied
		if (!catalogDb.findStage(params.p, params.s)) return notFound()
		return HttpResponse.json(catalogDb.appsOf(params.p, params.s).map(appOutput))
	}),

	http.post<Params>(apps, async ({ params, request }) => {
		const denied = requireSession()
		if (denied) return denied
		const body = await readBody(request, [
			'slug',
			'kind',
			'spec',
			'compose_yaml',
			'deploy_timeout_s',
		])
		if (body instanceof Response) return body
		try {
			const slug = String(body.slug ?? '')
			if (!SLUG_16.test(slug)) throw new Invalid('slug', slugMessage(slug, 16))
			const now = new Date().toISOString()
			const base = {
				project: params.p,
				stage: params.s,
				slug,
				config_changed_at: now,
				last_deployed_at: null,
				created_at: now,
			}
			let created: MockApp
			if (body.kind === 'service') {
				if (body.compose_yaml !== undefined && body.compose_yaml !== null) {
					throw new Invalid('compose_yaml', 'no aplica a apps de tipo service')
				}
				checkServiceSlug(params.p, params.s, slug)
				created = {
					...base,
					kind: 'service',
					spec: checkSpec(body.spec),
					deploy_timeout_s: 300,
				}
			} else if (body.kind === 'compose') {
				if (body.spec !== undefined && body.spec !== null) {
					throw new Invalid('spec', 'no aplica a apps de tipo compose')
				}
				const composeYaml = checkCompose(body.compose_yaml)
				checkComposeRules(params.p, params.s, slug, composeYaml)
				created = {
					...base,
					kind: 'compose',
					compose_yaml: composeYaml,
					deploy_timeout_s: 300,
				}
			} else {
				throw new Invalid('kind', 'debe ser service o compose')
			}
			if (body.deploy_timeout_s !== undefined) {
				created.deploy_timeout_s = checkTimeout(body.deploy_timeout_s)
			}
			if (!catalogDb.findStage(params.p, params.s)) return notFound()
			if (catalogDb.findApp(params.p, params.s, slug))
				return apiError(409, 'conflict', 'ya existe')
			catalogDb.state.apps.push(created)
			catalogDb.save()
			ingressDb.startEmpty(params.p, params.s, slug)
			return HttpResponse.json(appOutput(created), { status: 201 })
		} catch (error) {
			return failed(error)
		}
	}),

	http.get<Params>(app, ({ params }) => {
		const denied = requireSession()
		if (denied) return denied
		const found = catalogDb.findApp(params.p, params.s, params.a)
		return found ? HttpResponse.json(appOutput(found)) : notFound()
	}),

	http.patch<Params>(app, async ({ params, request }) => {
		const denied = requireSession()
		if (denied) return denied
		const body = await readBody(request, ['spec', 'compose_yaml', 'deploy_timeout_s'])
		if (body instanceof Response) return body
		try {
			const spec = body.spec === undefined || body.spec === null ? null : checkSpec(body.spec)
			const composeYaml =
				body.compose_yaml === undefined ? null : checkCompose(body.compose_yaml)
			const timeout =
				body.deploy_timeout_s === undefined ? null : checkTimeout(body.deploy_timeout_s)
			const found = catalogDb.findApp(params.p, params.s, params.a)
			if (!found) return notFound()
			if (
				(spec && found.kind !== 'service') ||
				(composeYaml !== null && found.kind !== 'compose')
			) {
				return apiError(422, 'kind_mismatch', 'el campo no corresponde al tipo de app')
			}
			if (composeYaml !== null) checkComposeRules(params.p, params.s, found.slug, composeYaml)
			if (spec) found.spec = spec
			if (composeYaml !== null) found.compose_yaml = composeYaml
			if (spec || composeYaml !== null) found.config_changed_at = new Date().toISOString()
			if (timeout !== null) found.deploy_timeout_s = timeout
			catalogDb.save()
			return HttpResponse.json(appOutput(found))
		} catch (error) {
			return failed(error)
		}
	}),

	http.delete<Params>(app, ({ params }) => {
		const denied = requireSession()
		if (denied) return denied
		if (!catalogDb.findApp(params.p, params.s, params.a)) return notFound()
		// `domains.app_id` es RESTRICT: primero se borran sus dominios. `list` siembra los del
		// diseño si nadie los pidió todavía: si no, borrar dependería de haber abierto Dominios.
		if (ingressDb.list(params.p, params.s, params.a).length > 0) {
			return apiError(409, 'not_empty', 'primero hay que borrar lo que contiene')
		}
		catalogDb.removeApp(params.p, params.s, params.a)
		catalogDb.save()
		return noContent()
	}),
]
