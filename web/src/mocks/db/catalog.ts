import { loadPersisted, savePersisted } from './persist'

// Proyectos, stages y apps del backend simulado, con la forma que devuelve la API
// (catalog/app/dto/dto.go). Los datos son los del diseño (`Main.dc.html`): auditfy,
// extractor y aumi-agents, cada uno con staging y production.

export interface MockProject {
	slug: string
	name: string
	created_at: string
}

export interface MockStage {
	project: string
	slug: string
	created_at: string
}

export interface MockHealthcheck {
	type: 'http' | 'bun-http' | 'command' | 'none'
	path?: string
	command?: string[]
	interval_s?: number
	timeout_s?: number
	retries?: number
	start_period_s?: number
}

/** El spec normalizado, como lo guarda el backend (`vo.AppSpec.JSON()`). */
export interface MockSpec {
	image: string
	port: number
	replicas: number
	command: string[]
	healthcheck: MockHealthcheck
	volumes: { name: string; mount: string }[]
	resources: { memory_mb?: number; cpus?: number } | null
}

export interface MockApp {
	project: string
	stage: string
	slug: string
	kind: 'service' | 'compose'
	spec?: MockSpec
	compose_yaml?: string
	deploy_timeout_s: number
	config_changed_at: string
	last_deployed_at: string | null
	created_at: string
}

export interface CatalogState {
	projects: MockProject[]
	stages: MockStage[]
	apps: MockApp[]
}

const STORAGE_KEY = 'catalog'

/** El compose de aumi-agents (`App-Compose.dc.html`), sin los `ports:` que catalog rechaza al guardar. */
export const MOCK_AGENTS_COMPOSE = `services:
  gateway:
    image: ghcr.io/aumi-app/agents-gateway:staging
    expose: ["8080"]
    environment:
      LLM_API_KEY: \${LLM_API_KEY}
      NATS_URL: \${NATS_URL:-nats://nats:4222}
    healthcheck:
      test: ["CMD", "wget", "-q", "-O", "/dev/null", "http://127.0.0.1:8080/health"]
      interval: 10s
  runner:
    image: ghcr.io/aumi-app/agents-runner:staging
    deploy:
      replicas: 2
    environment:
      WORKER_TOKEN: \${WORKER_TOKEN}
    volumes:
      - workspace:/workspace
  scheduler:
    image: ghcr.io/aumi-app/agents-scheduler:staging
    command: ["node", "dist/scheduler.js"]

volumes:
  workspace: {}
`

const HTTP_HEALTH: MockHealthcheck = {
	type: 'http',
	path: '/health',
	interval_s: 10,
	timeout_s: 5,
	retries: 3,
	start_period_s: 20,
}

function ago(minutes: number): string {
	return new Date(Date.now() - minutes * 60_000).toISOString()
}

const DAY = 24 * 60

function service(
	project: string,
	stage: string,
	slug: string,
	spec: MockSpec,
	deployedMinutesAgo: number | null,
	/** Un cambio de configuración posterior al deploy: la app queda con cambios pendientes. */
	changedMinutesAgo?: number,
): MockApp {
	return {
		project,
		stage,
		slug,
		kind: 'service',
		spec,
		deploy_timeout_s: 300,
		config_changed_at: ago(changedMinutesAgo ?? (deployedMinutesAgo ?? 30) + 1),
		last_deployed_at: deployedMinutesAgo === null ? null : ago(deployedMinutesAgo),
		created_at: ago(90 * DAY),
	}
}

function compose(project: string, stage: string, deployedMinutesAgo: number | null): MockApp {
	return {
		project,
		stage,
		slug: 'agents',
		kind: 'compose',
		compose_yaml: MOCK_AGENTS_COMPOSE.replaceAll(':staging', `:${stage}`),
		deploy_timeout_s: 600,
		config_changed_at: ago((deployedMinutesAgo ?? 30) + 1),
		last_deployed_at: deployedMinutesAgo === null ? null : ago(deployedMinutesAgo),
		created_at: ago(20 * DAY),
	}
}

function backendApi(image: string, replicas: number): MockSpec {
	return {
		image,
		port: 3000,
		replicas,
		command: [],
		healthcheck: HTTP_HEALTH,
		volumes: [{ name: 'uploads', mount: '/app/uploads' }],
		resources: { memory_mb: 512, cpus: 1 },
	}
}

const NATS: MockSpec = {
	image: 'nats:2.10-alpine',
	port: 4222,
	replicas: 1,
	command: ['--jetstream', '--store_dir', '/data'],
	healthcheck: { type: 'none' },
	volumes: [{ name: 'jetstream', mount: '/data' }],
	resources: null,
}

const REDIS: MockSpec = {
	image: 'redis:7.4-alpine',
	port: 6379,
	replicas: 1,
	command: [],
	healthcheck: {
		type: 'command',
		command: ['redis-cli', 'ping'],
		interval_s: 10,
		timeout_s: 3,
		retries: 5,
		start_period_s: 10,
	},
	volumes: [{ name: 'data', mount: '/data' }],
	resources: { memory_mb: 256 },
}

function extractor(image: string): MockSpec {
	return {
		image,
		port: 8080,
		replicas: 2,
		command: [],
		healthcheck: {
			type: 'bun-http',
			path: '/healthz',
			interval_s: 15,
			timeout_s: 5,
			retries: 3,
			start_period_s: 30,
		},
		volumes: [],
		resources: { memory_mb: 1024, cpus: 2 },
	}
}

function seed(): CatalogState {
	const created = ago(120 * DAY)
	return {
		projects: [
			{ slug: 'auditfy', name: 'Auditfy', created_at: created },
			{ slug: 'extractor', name: 'Extractor', created_at: created },
			{ slug: 'aumi-agents', name: 'Aumi Agents', created_at: created },
		],
		stages: ['auditfy', 'extractor', 'aumi-agents'].flatMap((project) => [
			{ project, slug: 'staging', created_at: created },
			{ project, slug: 'production', created_at: created },
		]),
		apps: [
			// api de staging: desplegada hace 12 min y con un cambio de después.
			service(
				'auditfy',
				'staging',
				'api',
				backendApi('ghcr.io/aumi-app/backend-app:sha-3f9c2e1', 2),
				12,
				5,
			),
			service('auditfy', 'staging', 'nats', NATS, 21 * DAY),
			service('auditfy', 'staging', 'redis', REDIS, 21 * DAY),
			service(
				'auditfy',
				'production',
				'api',
				backendApi('ghcr.io/aumi-app/backend-app:sha-a81d07b', 3),
				2 * DAY,
			),
			service('auditfy', 'production', 'nats', NATS, 34 * DAY),
			service('auditfy', 'production', 'redis', REDIS, 34 * DAY),
			service(
				'extractor',
				'staging',
				'api',
				extractor('ghcr.io/cpu-commits/extractor:sha-77e0c4a'),
				40,
			),
			service(
				'extractor',
				'production',
				'api',
				extractor('ghcr.io/cpu-commits/extractor:sha-5b19f3e'),
				6 * DAY,
			),
			compose('aumi-agents', 'staging', null),
			compose('aumi-agents', 'production', 3 * 60),
		],
	}
}

let state: CatalogState = loadPersisted<CatalogState>(STORAGE_KEY) ?? seed()

// --- Los servicios de la configuración (contrato §2.12) ------------------------
//
// Viven acá y no en handlers/catalog.ts porque los usan también los mocks de deploy (contra
// esos valida `images` y de ahí salen las refs de un primer deploy, §2.2) y de ingress.

/** Un puerto de `expose`: un entero o `N/tcp`; los rangos y los `${VAR}` no cuentan. */
function exposedPort(value: string): number | null {
	const match = /^(\d+)(\/tcp)?$/.exec(value.trim().replace(/^["']|["']$/g, ''))
	const port = match ? Number(match[1]) : Number.NaN
	return Number.isInteger(port) && port >= 1 && port <= 65535 ? port : null
}

function unquote(value: string): string {
	return value.trim().replace(/^["']|["']$/g, '')
}

/** Un servicio del compose: su nombre, los puertos de `expose` y su `image:` (o `''`). */
interface ComposeService {
	readonly name: string
	readonly ports: number[]
	readonly image: string
}

/**
 * Los servicios de un compose, como `compose.Services` del backend (contrato §2.12): por
 * nombre, con los puertos ordenados, y la `image:` de cada uno. Un parser de líneas alcanza
 * para los compose del mock (`expose: [..]` o una lista con `- `).
 */
function parseCompose(yaml: string): ComposeService[] {
	const services = new Map<string, { ports: Set<number>; image: string }>()
	let inServices = false
	let serviceIndent = -1
	let current: { ports: Set<number>; image: string } | null = null
	let keyIndent = -1
	let exposeIndent = -1
	const add = (value: string) => {
		const port = exposedPort(value)
		if (port !== null) current?.ports.add(port)
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
			current = { ports: new Set(), image: '' }
			services.set(text.replace(/:.*$/, ''), current)
			keyIndent = -1
			exposeIndent = -1
			continue
		}
		if (exposeIndent !== -1 && indent > exposeIndent && text.startsWith('- ')) {
			add(text.slice(2))
			continue
		}
		exposeIndent = -1
		if (keyIndent === -1) keyIndent = indent
		if (indent !== keyIndent || !current) continue
		const image = /^image:\s*(.+)$/.exec(text)
		if (image) current.image = unquote(image[1] ?? '')
		const expose = /^expose:\s*(.*)$/.exec(text)
		if (!expose) continue
		const inline = expose[1] ?? ''
		if (inline.startsWith('[')) inline.slice(1, -1).split(',').forEach(add)
		else exposeIndent = indent
	}
	return [...services]
		.map(([name, { ports, image }]) => ({
			name,
			ports: [...ports].sort((a, b) => a - b),
			image,
		}))
		.sort((a, b) => a.name.localeCompare(b.name))
}

/** Los servicios de un compose y los puertos de su `expose` (el `services` de §2.12). */
export function composeServices(yaml: string): { name: string; ports: number[] }[] {
	return parseCompose(yaml).map(({ name, ports }) => ({ name, ports }))
}

/** `services` del `AppOutput` (contrato §2.12): de la configuración guardada, no de Swarm. */
export function servicesOf(app: MockApp): { name: string; ports: number[] }[] {
	if (app.kind === 'compose') return composeServices(app.compose_yaml ?? '')
	const port = app.spec?.port ?? 0
	return [{ name: app.slug, ports: port > 0 ? [port] : [] }]
}

/** Lo que el deploy lee de la configuración de una app: su tipo y la ref de cada servicio. */
export interface ConfiguredApp {
	readonly kind: MockApp['kind']
	/** servicio → ref de la configuración (`spec.image` o el `image:` del compose), por nombre. */
	readonly images: Readonly<Record<string, string>>
	readonly lastDeployedAt: string | null
}

export function configuredApp(app: MockApp): ConfiguredApp {
	const images =
		app.kind === 'compose'
			? Object.fromEntries(
					parseCompose(app.compose_yaml ?? '').map((service) => [
						service.name,
						service.image,
					]),
				)
			: { [app.slug]: app.spec?.image ?? '' }
	return { kind: app.kind, images, lastDeployedAt: app.last_deployed_at }
}

function sameApp(app: MockApp, project: string, stage: string, slug: string): boolean {
	return app.project === project && app.stage === stage && app.slug === slug
}

export const catalogDb = {
	get state(): CatalogState {
		return state
	},

	/** Vuelve al estado inicial; los tests lo llaman en `beforeEach`. */
	reset(overrides: Partial<CatalogState> = {}): void {
		state = { ...seed(), ...overrides }
		savePersisted(STORAGE_KEY, state)
	},

	/** Después de modificar `state` desde un handler. */
	save(): void {
		savePersisted(STORAGE_KEY, state)
	},

	findProject(slug: string): MockProject | undefined {
		return state.projects.find((project) => project.slug === slug)
	},

	findStage(project: string, slug: string): MockStage | undefined {
		return state.stages.find((stage) => stage.project === project && stage.slug === slug)
	},

	findApp(project: string, stage: string, slug: string): MockApp | undefined {
		return state.apps.find((app) => sameApp(app, project, stage, slug))
	},

	/** El tipo y las refs de la configuración de una app, o undefined si no existe. */
	configured(project: string, stage: string, slug: string): ConfiguredApp | undefined {
		const app = this.findApp(project, stage, slug)
		return app ? configuredApp(app) : undefined
	},

	stagesOf(project: string): MockStage[] {
		return state.stages.filter((stage) => stage.project === project)
	},

	appsOf(project: string, stage: string): MockApp[] {
		return state.apps.filter((app) => app.project === project && app.stage === stage)
	},

	removeApp(project: string, stage: string, slug: string): void {
		state.apps = state.apps.filter((app) => !sameApp(app, project, stage, slug))
	},
}
