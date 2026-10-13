import {
	type AppChanges,
	type AppSpec,
	type AppSpecMessages,
	DEFAULT_HEALTHCHECK_TIMINGS,
	formatCommandLine,
	type Healthcheck,
	type HealthcheckType,
	isValidDeployTimeout,
	parseCommandLine,
	type ServiceApp,
	type SpecIssue,
	specIssues,
	specsEqual,
} from '../../../domain'

// Cada tarjeta de Configuración es un formulario propio con su "Guardar" (como el
// diseño). Guardar una arma el spec completo: el guardado, con lo de esa tarjeta
// encima. Este archivo es la traducción pura entre los valores del formulario (texto,
// como los escribe la persona) y el spec del dominio.

/** Los textos que no son reglas del spec, en el idioma de la UI. */
export interface SectionMessages extends AppSpecMessages {
	unclosedQuote: () => string
	deployTimeout: () => string
}

export interface SectionDef<V> {
	/** Los valores del formulario a partir de la app guardada. */
	values(app: ServiceApp): V
	/** El spec y el timeout que quedarían. `errors`: lo que ni siquiera se pudo armar. */
	apply(values: V, app: ServiceApp, messages: SectionMessages): SectionCandidate
	/** El campo del formulario que corresponde a un campo del dominio, o null si es de otra tarjeta. */
	fieldOf(domainField: string): string | null
	/**
	 * Si el campo del formulario está a la vista con estos valores (el path no se ve con un
	 * healthcheck de tipo comando). Sin esto, todos los de `fieldOf` lo están.
	 */
	visible?(field: string, values: V): boolean
}

export interface SectionCandidate {
	readonly spec: AppSpec
	readonly deployTimeoutSeconds: number
	readonly errors: Readonly<Record<string, string>>
}

export interface SectionCheck {
	/** Errores por campo del formulario. */
	readonly fields: Readonly<Record<string, string>>
	/** Problemas del spec guardado en campos de otras tarjetas. */
	readonly elsewhere: readonly SpecIssue[]
}

// Un número escrito a mano: vacío o mal escrito es NaN, y la regla lo rechaza con su
// mensaje de rango ("Entre 1 y 65535").
function toNumber(raw: string): number {
	const text = raw.trim()
	return text === '' ? Number.NaN : Number(text)
}

// Un límite opcional: vacío es "sin límite".
function toOptionalNumber(raw: string): number | null {
	return raw.trim() === '' ? null : toNumber(raw)
}

const text = (value: number | null): string => (value === null ? '' : String(value))

// --- Imagen -------------------------------------------------------------------------------

export interface ImageValues {
	image: string
}

export const imageSection: SectionDef<ImageValues> = {
	values: (app) => ({ image: app.spec.image }),
	apply: (values, app) => ({
		spec: { ...app.spec, image: values.image.trim() },
		deployTimeoutSeconds: app.deployTimeoutSeconds,
		errors: {},
	}),
	fieldOf: (field) => (field === 'image' ? 'image' : null),
}

// --- Ejecución ----------------------------------------------------------------------------

export interface RunValues {
	port: string
	replicas: string
	command: string
}

export const runSection: SectionDef<RunValues> = {
	values: (app) => ({
		port: String(app.spec.port),
		replicas: String(app.spec.replicas),
		command: formatCommandLine(app.spec.command),
	}),
	apply: (values, app, messages) => {
		const command = parseCommandLine(values.command)
		return {
			spec: {
				...app.spec,
				port: toNumber(values.port),
				replicas: toNumber(values.replicas),
				command: command ?? app.spec.command,
			},
			deployTimeoutSeconds: app.deployTimeoutSeconds,
			errors: command ? {} : { command: messages.unclosedQuote() },
		}
	},
	fieldOf: (field) => (['port', 'replicas', 'command'].includes(field) ? field : null),
}

// --- Healthcheck --------------------------------------------------------------------------

export interface HealthValues {
	type: HealthcheckType
	path: string
	command: string
	intervalSeconds: string
	timeoutSeconds: string
	retries: string
	startPeriodSeconds: string
}

const HEALTH_FIELDS: Readonly<Record<string, string>> = {
	healthcheck: 'type',
	'healthcheck.type': 'type',
	'healthcheck.path': 'path',
	'healthcheck.command': 'command',
	'healthcheck.intervalSeconds': 'intervalSeconds',
	'healthcheck.timeoutSeconds': 'timeoutSeconds',
	'healthcheck.retries': 'retries',
	'healthcheck.startPeriodSeconds': 'startPeriodSeconds',
}

export const healthSection: SectionDef<HealthValues> = {
	values: (app) => {
		const hc = app.spec.healthcheck
		// Al pasar de "Ninguno" a otro tipo, los tiempos arrancan con los del backend.
		const timings = hc.type === 'none' ? DEFAULT_HEALTHCHECK_TIMINGS : hc
		return {
			type: hc.type,
			path: hc.type === 'http' || hc.type === 'bun-http' ? hc.path : '/health',
			command: hc.type === 'command' ? formatCommandLine(hc.command) : '',
			intervalSeconds: String(timings.intervalSeconds),
			timeoutSeconds: String(timings.timeoutSeconds),
			retries: String(timings.retries),
			startPeriodSeconds: String(timings.startPeriodSeconds),
		}
	},
	apply: (values, app, messages) => {
		const { healthcheck, errors } = healthcheckFrom(values, messages)
		return {
			spec: { ...app.spec, healthcheck },
			deployTimeoutSeconds: app.deployTimeoutSeconds,
			errors,
		}
	},
	fieldOf: (field) => HEALTH_FIELDS[field] ?? null,
	visible: (field, values) => {
		if (field === 'type') return true
		if (field === 'path') return values.type === 'http' || values.type === 'bun-http'
		if (field === 'command') return values.type === 'command'
		return values.type !== 'none'
	},
}

/** El healthcheck que describen los valores (también para la vista previa). */
export function healthcheckFrom(
	values: HealthValues,
	messages: Pick<SectionMessages, 'unclosedQuote'>,
): { healthcheck: Healthcheck; errors: Record<string, string> } {
	if (values.type === 'none') return { healthcheck: { type: 'none' }, errors: {} }
	const timings = {
		intervalSeconds: toNumber(values.intervalSeconds),
		timeoutSeconds: toNumber(values.timeoutSeconds),
		retries: toNumber(values.retries),
		startPeriodSeconds: toNumber(values.startPeriodSeconds),
	}
	if (values.type === 'command') {
		const command = parseCommandLine(values.command)
		return {
			healthcheck: { type: 'command', command: command ?? [], ...timings },
			errors: command ? {} : { command: messages.unclosedQuote() },
		}
	}
	return { healthcheck: { type: values.type, path: values.path.trim(), ...timings }, errors: {} }
}

// --- Volúmenes ----------------------------------------------------------------------------

export interface VolumeRow {
	name: string
	mount: string
}

export interface VolumesValues {
	volumes: VolumeRow[]
}

export const EMPTY_VOLUME: VolumeRow = { name: '', mount: '' }

export const volumesSection: SectionDef<VolumesValues> = {
	values: (app) => ({ volumes: app.spec.volumes.map((volume) => ({ ...volume })) }),
	apply: (values, app) => ({
		spec: {
			...app.spec,
			volumes: values.volumes.map((volume) => ({
				name: volume.name.trim(),
				mount: volume.mount.trim(),
			})),
		},
		deployTimeoutSeconds: app.deployTimeoutSeconds,
		errors: {},
	}),
	fieldOf: (field) => (field === 'volumes' || field.startsWith('volumes[') ? field : null),
	visible: (field, values) => {
		const row = /^volumes\[(\d+)\]/.exec(field)
		return row === null || Number(row[1]) < values.volumes.length
	},
}

// --- Recursos -----------------------------------------------------------------------------

export interface ResourcesValues {
	memoryMb: string
	cpus: string
	deployTimeoutSeconds: string
}

const RESOURCE_FIELDS: Readonly<Record<string, string>> = {
	resources: 'memoryMb',
	'resources.memoryMb': 'memoryMb',
	'resources.cpus': 'cpus',
	deployTimeoutSeconds: 'deployTimeoutSeconds',
}

export const resourcesSection: SectionDef<ResourcesValues> = {
	values: (app) => ({
		memoryMb: text(app.spec.resources?.memoryMb ?? null),
		cpus: text(app.spec.resources?.cpus ?? null),
		deployTimeoutSeconds: String(app.deployTimeoutSeconds),
	}),
	apply: (values, app, messages) => {
		const memoryMb = toOptionalNumber(values.memoryMb)
		const cpus = toOptionalNumber(values.cpus)
		const deployTimeoutSeconds = toNumber(values.deployTimeoutSeconds)
		return {
			spec: {
				...app.spec,
				resources: memoryMb === null && cpus === null ? null : { memoryMb, cpus },
			},
			deployTimeoutSeconds,
			errors: isValidDeployTimeout(deployTimeoutSeconds)
				? {}
				: { deployTimeoutSeconds: messages.deployTimeout() },
		}
	},
	fieldOf: (field) => RESOURCE_FIELDS[field] ?? null,
}

export type CardId = 'image' | 'run' | 'health' | 'volumes' | 'resources'

/** La tarjeta donde se edita un campo del dominio (`port` → Ejecución), o null (`spec`). */
export function cardOf(field: string): CardId | null {
	if (imageSection.fieldOf(field) !== null) return 'image'
	if (runSection.fieldOf(field) !== null) return 'run'
	if (healthSection.fieldOf(field) !== null) return 'health'
	if (volumesSection.fieldOf(field) !== null) return 'volumes'
	if (resourcesSection.fieldOf(field) !== null) return 'resources'
	return null
}

// --- Validar y comparar -------------------------------------------------------------------

/** Los errores de una tarjeta, con el spec completo validado por las reglas del dominio. */
export function checkSection<V>(
	def: SectionDef<V>,
	values: V,
	app: ServiceApp,
	messages: SectionMessages,
): SectionCheck {
	const candidate = def.apply(values, app, messages)
	const fields: Record<string, string> = { ...candidate.errors }
	const elsewhere: SpecIssue[] = []
	for (const issue of specIssues(candidate.spec, messages)) {
		const field = def.fieldOf(issue.field)
		if (field === null) elsewhere.push(issue)
		else fields[field] ??= issue.message
	}
	return { fields, elsewhere }
}

/**
 * Lo que hay que mandar: solo lo que cambió. Mandar el spec igual marcaría "cambios
 * pendientes" sin haber cambiado nada. `null` si no hay nada.
 */
export function sectionChanges<V>(
	def: SectionDef<V>,
	values: V,
	app: ServiceApp,
	messages: SectionMessages,
): AppChanges | null {
	const candidate = def.apply(values, app, messages)
	const changes: AppChanges = {
		...(specsEqual(candidate.spec, app.spec) ? {} : { spec: candidate.spec }),
		...(candidate.deployTimeoutSeconds === app.deployTimeoutSeconds
			? {}
			: { deployTimeoutSeconds: candidate.deployTimeoutSeconds }),
	}
	return Object.keys(changes).length > 0 ? changes : null
}

/**
 * Dónde mostrar un error del backend: el campo del formulario si está a la vista, o null
 * (va arriba de la tarjeta, en su `formError`).
 */
export function visibleFieldOf<V>(
	def: SectionDef<V>,
	domainField: string,
	values: V,
): string | null {
	const field = def.fieldOf(domainField)
	if (field === null) return null
	return def.visible === undefined || def.visible(field, values) ? field : null
}

/** El valor de un campo del formulario por su nombre (`volumes[1].mount`). */
export function valueAt(values: unknown, field: string): unknown {
	return field
		.split(/[.[\]]/)
		.filter(Boolean)
		.reduce<unknown>(
			(current, key) =>
				current !== null && typeof current === 'object'
					? (current as Record<string, unknown>)[key]
					: undefined,
			values,
		)
}
