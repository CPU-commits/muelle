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

/** El compose de aumi-agents (`App-Compose.dc.html`), sin los `ports:` que la Fase 2 rechaza. */
export const MOCK_AGENTS_COMPOSE = `services:
  gateway:
    image: ghcr.io/aumi-app/agents-gateway:staging
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
