import { loadPersisted, savePersisted } from './persist'

// Las variables de stages y apps del backend simulado. Guarda el valor de las secretas
// (como el backend, que las tiene cifradas); los handlers las devuelven enmascaradas.

export interface MockEnvVar {
	key: string
	value: string
	secret: boolean
	updatedAt: string
}

/** El dueño de un set: un stage, o una app si viene `app`. */
export interface MockEnvTarget {
	project: string
	stage: string
	app?: string
}

export interface SecretsState {
	/** Por `p/s` (stage) o `p/s/a` (app). Un dueño sin entrada no tiene variables. */
	owners: Record<string, MockEnvVar[]>
}

/** Recibe al dueño que cambió (las apps quedan con cambios pendientes). */
export type EnvChangeListener = (target: MockEnvTarget) => void
export type EnvOwnerResolver = (target: MockEnvTarget) => boolean

const STORAGE_KEY = 'secrets'
const DAY_MS = 24 * 60 * 60 * 1000

function ago(ms: number): string {
	return new Date(Date.now() - ms).toISOString()
}

function plain(key: string, value: string, age: number): MockEnvVar {
	return { key, value, secret: false, updatedAt: ago(age) }
}

function secret(key: string, value: string, age: number): MockEnvVar {
	return { key, value, secret: true, updatedAt: ago(age) }
}

// Los datos de App-Variables.dc.html: el stage staging de auditfy y su app api.
function seed(): SecretsState {
	return {
		owners: {
			'auditfy/staging': [
				plain('NODE_ENV', 'staging', 40 * DAY_MS),
				plain('TZ', 'America/Santiago', 40 * DAY_MS),
				plain('LOG_LEVEL', 'debug', 30 * DAY_MS),
				plain('NATS_URL', 'nats://nats:4222', 30 * DAY_MS),
				plain('REDIS_URL', 'redis://redis:6379', 30 * DAY_MS),
				secret('DATABASE_URL', 'postgres://auditfy:demo@db:5432/auditfy', 30 * DAY_MS),
				secret('MONGO_URL', 'mongodb://auditfy:demo@mongo:27017/auditfy', 30 * DAY_MS),
				secret('SENTRY_DSN', 'https://demo@sentry.io/1', 30 * DAY_MS),
			],
			'auditfy/staging/api': [
				plain('PORT', '3000', 21 * DAY_MS),
				plain('LOG_LEVEL', 'info', 21 * DAY_MS),
				plain('CORS_ORIGINS', 'https://staging.tudominio.cl', 9 * DAY_MS),
				plain('S3_BUCKET', 'auditfy-staging-uploads', 9 * DAY_MS),
				secret('S3_ACCESS_KEY_ID', 'AKIADEMO', 5 * 60 * 1000),
				secret('S3_SECRET_ACCESS_KEY', 'demo-secret-access-key', 5 * 60 * 1000),
				secret('JWT_SECRET', 'demo-jwt-secret', 21 * DAY_MS),
			],
			'auditfy/production': [
				plain('NODE_ENV', 'production', 60 * DAY_MS),
				plain('TZ', 'America/Santiago', 60 * DAY_MS),
				secret('DATABASE_URL', 'postgres://auditfy:prod@db:5432/auditfy', 60 * DAY_MS),
			],
		},
	}
}

let state: SecretsState = loadPersisted<SecretsState>(STORAGE_KEY) ?? seed()
let resolveOwner: EnvOwnerResolver | null = null
let onChange: EnvChangeListener | null = null

export function envOwnerId(target: MockEnvTarget): string {
	const stage = `${target.project}/${target.stage}`
	return target.app ? `${stage}/${target.app}` : stage
}

export const secretsDb = {
	get state(): SecretsState {
		return state
	},

	/** Vuelve al estado inicial; los tests lo llaman en `beforeEach`. */
	reset(overrides: Partial<SecretsState> = {}): void {
		state = { ...seed(), ...overrides }
		savePersisted(STORAGE_KEY, state)
	},

	commit(): void {
		savePersisted(STORAGE_KEY, state)
	},

	/**
	 * Por defecto todo dueño existe: los proyectos, stages y apps viven en el mock de
	 * catalog. `handlers/secrets.ts` (o un test) conecta acá la búsqueda de verdad; `null`
	 * vuelve al defecto.
	 */
	setOwnerResolver(resolver: EnvOwnerResolver | null): void {
		resolveOwner = resolver
	},

	/** Para marcar las apps con cambios pendientes en el mock de catalog (`touchConfig`). */
	setChangeListener(listener: EnvChangeListener | null): void {
		onChange = listener
	},

	ownerExists(target: MockEnvTarget): boolean {
		return resolveOwner ? resolveOwner(target) : true
	},

	vars(target: MockEnvTarget): MockEnvVar[] {
		return state.owners[envOwnerId(target)] ?? []
	},

	/** Guarda el set nuevo del dueño y avisa del cambio. */
	save(target: MockEnvTarget, vars: MockEnvVar[]): void {
		state.owners = { ...state.owners, [envOwnerId(target)]: vars }
		this.commit()
		onChange?.(target)
	},
}
