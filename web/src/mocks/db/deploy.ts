import { loadPersisted, savePersisted } from './persist'

// Estado, deploys y logs (Fase 2) del backend simulado.
//
// Un deploy simulado no tiene timers: su estado y su log se CALCULAN a partir de la hora.
// Cada uno guarda cuándo se creó, cuándo lo toma el worker (`startAt`) y su guion
// (`scenario`); `view(…, now)` dice en qué paso va. Así el polling (`?offset`) y el SSE
// ven exactamente lo mismo, y un F5 no lo pierde (se guarda en sessionStorage).
//
// Guiones: `success` (queued → running → succeeded), `healthcheck` (falla el
// healthcheck, con la salida de la tarea y un secreto enmascarado), `image_missing` (no
// se resuelve la imagen). Un deploy en cola que recibe otro encima queda `superseded`.
//
// Para probar desde el panel: "Desplegar" con una imagen que contenga `fail` o `broken`
// termina `failed` por healthcheck; con `missing`, por imagen inexistente.

export type MockScenario = 'success' | 'healthcheck' | 'image_missing'
export type MockTrigger = 'panel' | 'api' | 'rollback'
export type MockStatus = 'queued' | 'running' | 'succeeded' | 'failed' | 'superseded'

export interface MockDeploy {
	id: number
	createdAt: number
	/** Cuándo lo toma el worker de la app (después del que está corriendo). */
	startAt: number
	/** Multiplica los tiempos del guion: el historial dura ~1 min, uno nuevo ~9 s. */
	scale: number
	scenario: MockScenario
	/** servicio → ref (sin digest; el digest sale de la ref). */
	images: Record<string, string>
	trigger: MockTrigger
	actorType: 'user' | 'token'
	actorName: string
	rollbackOf?: number
	supersededBy?: number
	supersededAt?: number
}

export interface MockApp {
	project: string
	stage: string
	app: string
	services: string[]
	replicas: number
	deploys: MockDeploy[]
}

interface DeployState {
	apps: Record<string, MockApp>
	/** Escala de los deploys nuevos: 1 en el navegador, mucho menos en los tests. */
	liveScale: number
	/** Si al sembrar una app se crea un deploy en curso (el #144 del diseño). */
	seedLive: boolean
}

export interface DeployView {
	status: MockStatus
	log: string
	startedAt: number | null
	finishedAt: number | null
	error: string | null
	/** servicio → ref@digest (o la ref sola si todavía no se resolvió). */
	images: Record<string, string>
}

interface Step {
	at: number
	text: string
}

interface Script {
	steps: Step[]
	status: 'succeeded' | 'failed'
	error: string | null
	/** Desde qué momento del guion la imagen ya tiene digest. */
	resolvedAt: number | null
	/** Desde qué momento hay una tarea nueva arrancando. */
	newTaskAt: number | null
}

const STORAGE_KEY = 'deploy'
const SECOND = 1000
const MINUTE = 60 * SECOND
const HOUR = 60 * MINUTE
const DAY = 24 * HOUR
/** Lo que espera en cola un deploy nuevo antes de que el worker lo tome. */
const QUEUE_MS = 1500
const HISTORY_SCALE = 6
const MEMORY_LIMIT = 512 * 1024 * 1024

function seed(overrides: Partial<DeployState> = {}): DeployState {
	return { apps: {}, liveScale: 1, seedLive: true, ...overrides }
}

let state: DeployState = loadPersisted<DeployState>(STORAGE_KEY) ?? seed()

function commit(): void {
	savePersisted(STORAGE_KEY, state)
}

export function appKey(project: string, stage: string, app: string): string {
	return `${project}/${stage}/${app}`
}

// Un hash chico y estable para inventar digests e ids de tarea que no cambien entre
// llamadas (FNV-1a de 32 bits, encadenado hasta el largo pedido).
export function hashHex(input: string, length: number): string {
	let out = ''
	let seedValue = 0x811c9dc5
	while (out.length < length) {
		let hash = seedValue
		for (let index = 0; index < input.length; index += 1) {
			hash ^= input.charCodeAt(index)
			hash = Math.imul(hash, 0x01000193) >>> 0
		}
		out += hash.toString(16).padStart(8, '0')
		seedValue = (hash ^ out.length) >>> 0
	}
	return out.slice(0, length)
}

export function digestOf(ref: string): string {
	return `sha256:${hashHex(ref, 64)}`
}

function shortDigest(digest: string): string {
	const hash = digest.slice('sha256:'.length)
	return `sha256:${hash.slice(0, 8)}…${hash.slice(-8)}`
}

function iso(ms: number): string {
	return new Date(ms).toISOString()
}

function stackOf(app: MockApp): string {
	return `${app.project}_${app.stage}_${app.app}`
}

function triggerLabel(trigger: MockTrigger): string {
	return trigger === 'api' ? 'api' : trigger
}

function secondsLabel(ms: number): string {
	const seconds = Math.round(ms / SECOND)
	return seconds < 60 ? `${seconds} s` : `${Math.floor(seconds / 60)} min ${seconds % 60} s`
}

function scriptFor(app: MockApp, deploy: MockDeploy): Script {
	const services = Object.keys(deploy.images)
	const stack = stackOf(app)
	const first = services[0] ?? app.app
	const s = (ms: number) => ms * deploy.scale
	const steps: Step[] = []
	if (deploy.rollbackOf !== undefined) {
		const tags = services.map(
			(service) => `${service}:${deploy.images[service]?.split(':').pop()}`,
		)
		steps.push({ at: 0, text: `imágenes de #${deploy.rollbackOf}: ${tags.join(', ')}` })
	}
	steps.push({ at: 0, text: 'resolviendo imágenes' })

	if (deploy.scenario === 'image_missing') {
		const ref = deploy.images[first] ?? ''
		steps.push({ at: s(900), text: `error: ${ref}: manifest unknown` })
		steps.push({ at: s(1000), text: `deploy fallido: no se pudo resolver la imagen ${ref}` })
		return {
			steps,
			status: 'failed',
			error: `no se pudo resolver la imagen ${ref}`,
			resolvedAt: null,
			newTaskAt: null,
		}
	}

	for (const service of services) {
		const ref = deploy.images[service] ?? ''
		steps.push({ at: s(700), text: `${service}  ${ref} → ${shortDigest(digestOf(ref))}` })
	}
	const plural = services.length === 1 ? 'servicio' : 'servicios'
	steps.push({
		at: s(1000),
		text: `render ok · ${services.length} ${plural} · 14 variables (6 secretas)`,
	})
	steps.push({
		at: s(1300),
		text: `red ${app.project}_${app.stage} lista · muelle-caddy conectado`,
	})
	steps.push({
		at: s(1600),
		text: `$ docker stack deploy --with-registry-auth --prune --detach=true -c - ${stack}`,
	})
	for (const service of services) {
		steps.push({ at: s(2200), text: `Updating service ${stack}_${service}` })
	}
	steps.push({ at: s(2500), text: 'esperando convergencia · timeout 300 s' })
	steps.push({ at: s(3100), text: `${first}.1 tarea nueva · starting` })

	if (deploy.scenario === 'healthcheck') {
		const failAt = s(7000)
		steps.push({ at: s(6500), text: `${first}.1 unhealthy · 3 healthchecks fallidos` })
		steps.push({ at: failAt, text: 'UpdateStatus: rollback_started' })
		steps.push({ at: failAt, text: 'task: non-zero exit (1)' })
		steps.push({
			at: failAt,
			text: `conectando a postgres://${app.project}:***@10.0.3.4:5432/${app.project}`,
		})
		steps.push({ at: failAt, text: 'error: relation "findings_v2" does not exist' })
		steps.push({ at: failAt, text: 'proceso terminado con código 1' })
		steps.push({
			at: s(8400),
			text: 'deploy fallido: la versión nueva no pasó el healthcheck',
		})
		return {
			steps,
			status: 'failed',
			error: 'la versión nueva no pasó el healthcheck',
			resolvedAt: s(700),
			newTaskAt: s(3100),
		}
	}

	const total = app.replicas
	steps.push({ at: s(5300), text: `${first}.1 healthy · 1 de ${total} actualizadas` })
	if (total >= 2) {
		steps.push({ at: s(5700), text: `${first}.2 tarea nueva · starting` })
		steps.push({ at: s(7900), text: `${first}.2 healthy · 2 de ${total} actualizadas` })
	}
	steps.push({ at: s(8500), text: 'UpdateStatus: completed' })
	steps.push({ at: s(8800), text: 'rutas de Caddy enviadas · 3 dominios' })
	steps.push({ at: s(9100), text: `deploy exitoso en ${secondsLabel(s(9100))}` })
	return { steps, status: 'succeeded', error: null, resolvedAt: s(700), newTaskAt: s(3100) }
}

function scriptDuration(script: Script): number {
	return script.steps.reduce((max, step) => Math.max(max, step.at), 0)
}

function queuedLine(deploy: MockDeploy): string {
	const who = deploy.actorType === 'token' ? `token ${deploy.actorName}` : deploy.actorName
	return `${iso(deploy.createdAt)} deploy #${deploy.id} encolado · ${triggerLabel(deploy.trigger)} · ${who}`
}

function withDigests(images: Record<string, string>): Record<string, string> {
	return Object.fromEntries(
		Object.entries(images).map(([service, ref]) => [service, `${ref}@${digestOf(ref)}`]),
	)
}

export function view(app: MockApp, deploy: MockDeploy, now: number): DeployView {
	const lines = [queuedLine(deploy)]
	if (deploy.supersededAt !== undefined && deploy.supersededAt <= now) {
		lines.push(
			`${iso(deploy.supersededAt)} reemplazado por #${deploy.supersededBy} antes de empezar`,
		)
		return {
			status: 'superseded',
			log: `${lines.join('\n')}\n`,
			startedAt: null,
			finishedAt: deploy.supersededAt,
			error: null,
			images: { ...deploy.images },
		}
	}
	const elapsed = now - deploy.startAt
	if (elapsed < 0) {
		return {
			status: 'queued',
			log: `${lines.join('\n')}\n`,
			startedAt: null,
			finishedAt: null,
			error: null,
			images: { ...deploy.images },
		}
	}
	const script = scriptFor(app, deploy)
	const duration = scriptDuration(script)
	for (const step of script.steps) {
		if (step.at <= elapsed) lines.push(`${iso(deploy.startAt + step.at)} ${step.text}`)
	}
	const resolved = script.resolvedAt !== null && elapsed >= script.resolvedAt
	const images = resolved ? withDigests(deploy.images) : { ...deploy.images }
	if (elapsed < duration) {
		return {
			status: 'running',
			log: `${lines.join('\n')}\n`,
			startedAt: deploy.startAt,
			finishedAt: null,
			error: null,
			images,
		}
	}
	return {
		status: script.status,
		log: `${lines.join('\n')}\n`,
		startedAt: deploy.startAt,
		finishedAt: deploy.startAt + duration,
		error: script.error,
		images,
	}
}

/** Cuándo termina (o terminó) de correr un deploy que no fue reemplazado. */
function finishOf(app: MockApp, deploy: MockDeploy): number {
	return deploy.startAt + scriptDuration(scriptFor(app, deploy))
}

// --- Siembra ----------------------------------------------------------------

interface SeedEntry {
	id: number
	ago: number
	scenario: MockScenario
	tag: string
	trigger: MockTrigger
	actor: [MockDeploy['actorType'], string]
	rollbackOf?: number
}

const RICH_HISTORY: SeedEntry[] = [
	{
		id: 139,
		ago: 3 * DAY,
		scenario: 'success',
		tag: 'sha-b27c19e',
		trigger: 'rollback',
		actor: ['user', 'felipe'],
		rollbackOf: 137,
	},
	{
		id: 140,
		ago: DAY,
		scenario: 'success',
		tag: 'sha-e52b8f0',
		trigger: 'api',
		actor: ['token', 'github-actions'],
	},
	{
		id: 141,
		ago: 62 * MINUTE,
		scenario: 'success',
		tag: 'sha-6a0f2d3',
		trigger: 'api',
		actor: ['token', 'github-actions'],
	},
	{
		id: 142,
		ago: 61 * MINUTE,
		scenario: 'healthcheck',
		tag: 'sha-9d41b7c',
		trigger: 'api',
		actor: ['token', 'github-actions'],
	},
	{
		id: 143,
		ago: 12 * MINUTE,
		scenario: 'success',
		tag: 'sha-3f9c2e1',
		trigger: 'api',
		actor: ['token', 'github-actions'],
	},
]

// Apps que en el diseño son de una imagen pública con un solo deploy (Main.dc.html).
const SINGLE_DEPLOY: Record<string, string> = {
	nats: 'nats:2.10-alpine',
	redis: 'redis:7.4-alpine',
}

function neverDeployed(slug: string): boolean {
	return slug.startsWith('nueva') || slug.startsWith('new') || slug === 'agents'
}

function seedApp(project: string, stage: string, slug: string, now: number): MockApp {
	const app: MockApp = { project, stage, app: slug, services: [slug], replicas: 2, deploys: [] }
	if (neverDeployed(slug)) return app

	const single = SINGLE_DEPLOY[slug]
	if (single) {
		app.replicas = 1
		const createdAt = now - 21 * DAY
		app.deploys.push({
			id: 12,
			createdAt,
			startAt: createdAt + 800 * HISTORY_SCALE,
			scale: HISTORY_SCALE,
			scenario: 'success',
			images: { [slug]: single },
			trigger: 'panel',
			actorType: 'user',
			actorName: 'felipe',
		})
		return app
	}

	const image = `ghcr.io/${project}/${slug}`
	for (const entry of RICH_HISTORY) {
		const createdAt = now - entry.ago
		app.deploys.push({
			id: entry.id,
			createdAt,
			startAt: createdAt + 800 * HISTORY_SCALE,
			scale: HISTORY_SCALE,
			scenario: entry.scenario,
			images: { [slug]: `${image}:${entry.tag}` },
			trigger: entry.trigger,
			actorType: entry.actor[0],
			actorName: entry.actor[1],
			...(entry.rollbackOf !== undefined ? { rollbackOf: entry.rollbackOf } : {}),
		})
	}
	// #141 quedó en cola y #142 lo reemplazó antes de empezar.
	const superseded = app.deploys.find((deploy) => deploy.id === 141)
	if (superseded) {
		superseded.supersededBy = 142
		superseded.supersededAt = now - 61 * MINUTE
	}
	if (state.seedLive) {
		app.deploys.push({
			id: 144,
			createdAt: now,
			startAt: now + QUEUE_MS * state.liveScale,
			scale: state.liveScale,
			scenario: 'success',
			images: { [slug]: `${image}:sha-3f9c2e1` },
			trigger: 'panel',
			actorType: 'user',
			actorName: 'felipe',
		})
	}
	return app
}

// --- Operaciones ------------------------------------------------------------

export interface CreateInput {
	images: Record<string, string>
	trigger: MockTrigger
	actorName: string
	rollbackOf?: number
	scenario?: MockScenario
}

function scenarioFor(images: Record<string, string>): MockScenario {
	const refs = Object.values(images).join(' ')
	if (/missing|noexiste/i.test(refs)) return 'image_missing'
	if (/fail|broken/i.test(refs)) return 'healthcheck'
	return 'success'
}

export const deployDb = {
	get state(): DeployState {
		return state
	},

	/** Vuelve al estado inicial; los tests lo llaman en `beforeEach`. */
	reset(overrides: Partial<DeployState> = {}): void {
		state = seed(overrides)
		commit()
	},

	commit,

	/** La app (sembrada la primera vez que se pide). */
	app(project: string, stage: string, slug: string, now = Date.now()): MockApp {
		const key = appKey(project, stage, slug)
		let app = state.apps[key]
		if (!app) {
			app = seedApp(project, stage, slug, now)
			state.apps[key] = app
			commit()
		}
		return app
	},

	find(app: MockApp, id: number): MockDeploy | undefined {
		return app.deploys.find((deploy) => deploy.id === id)
	},

	view,

	/** El succeeded más reciente a la hora `now`. */
	current(app: MockApp, now: number): MockDeploy | undefined {
		return [...app.deploys]
			.sort((a, b) => b.id - a.id)
			.find((deploy) => view(app, deploy, now).status === 'succeeded')
	},

	/** Encola un deploy: reemplaza al que está en cola y espera al que está corriendo. */
	create(app: MockApp, input: CreateInput, now = Date.now()): MockDeploy {
		const id = Math.max(0, ...app.deploys.map((deploy) => deploy.id)) + 1
		let startAt = now + QUEUE_MS * state.liveScale
		for (const deploy of app.deploys) {
			const status = view(app, deploy, now).status
			if (status === 'queued') {
				deploy.supersededBy = id
				deploy.supersededAt = now
			} else if (status === 'running') {
				startAt = Math.max(startAt, finishOf(app, deploy))
			}
		}
		const deploy: MockDeploy = {
			id,
			createdAt: now,
			startAt,
			scale: state.liveScale,
			scenario: input.scenario ?? scenarioFor(input.images),
			images: input.images,
			trigger: input.trigger,
			actorType: 'user',
			actorName: input.actorName,
			...(input.rollbackOf !== undefined ? { rollbackOf: input.rollbackOf } : {}),
		}
		app.deploys.push(deploy)
		// Retención: los últimos 20 por app (spec §5.2 regla 7).
		app.deploys = [...app.deploys].sort((a, b) => a.id - b.id).slice(-20)
		commit()
		return deploy
	},

	/** Las imágenes del succeeded anterior al actual, o null si no hay (→ 409). */
	rollbackSource(app: MockApp, now: number): MockDeploy | null {
		const succeeded = [...app.deploys]
			.sort((a, b) => b.id - a.id)
			.filter((deploy) => view(app, deploy, now).status === 'succeeded')
		return succeeded[1] ?? null
	},

	/** Si hay un deploy corriendo con una tarea nueva arrancando. */
	startingTask(app: MockApp, now: number): boolean {
		return app.deploys.some((deploy) => {
			const current = view(app, deploy, now)
			if (current.status !== 'running') return false
			const script = scriptFor(app, deploy)
			return script.newTaskAt !== null && now - deploy.startAt >= script.newTaskAt
		})
	},

	memoryLimit: MEMORY_LIMIT,
}
