import { z } from 'zod'
import { InvalidAppSpec } from '../errors/catalogErrors'
import { isValidVolumeName } from './slugs'

// El spec de una app de formulario (§6.1 del spec, `vo.AppSpec` del backend). Las reglas
// de abajo replican `appspec.go`: el backend vuelve a validar, pero el formulario avisa
// antes de mandar y con el error en su campo.

export const HEALTHCHECK_TYPES = ['http', 'bun-http', 'command', 'none'] as const
export type HealthcheckType = (typeof HEALTHCHECK_TYPES)[number]

export interface HealthcheckTimings {
	readonly intervalSeconds: number
	readonly timeoutSeconds: number
	readonly retries: number
	readonly startPeriodSeconds: number
}

export type Healthcheck =
	| { readonly type: 'none' }
	| ({ readonly type: 'http' | 'bun-http'; readonly path: string } & HealthcheckTimings)
	| ({ readonly type: 'command'; readonly command: readonly string[] } & HealthcheckTimings)

export interface Volume {
	readonly name: string
	readonly mount: string
}

/** Límites por réplica. `null` es "sin límite" (el backend guarda 0). */
export interface Resources {
	readonly memoryMb: number | null
	readonly cpus: number | null
}

export interface AppSpec {
	readonly image: string
	readonly port: number
	readonly replicas: number
	/** Vacío: el CMD de la imagen. */
	readonly command: readonly string[]
	readonly healthcheck: Healthcheck
	readonly volumes: readonly Volume[]
	readonly resources: Resources | null
}

/** Lo que el backend completa cuando un healthcheck que no es `none` no trae tiempos. */
export const DEFAULT_HEALTHCHECK_TIMINGS: HealthcheckTimings = {
	intervalSeconds: 10,
	timeoutSeconds: 5,
	retries: 3,
	startPeriodSeconds: 20,
}

export const SPEC_LIMITS = {
	imageMaxLength: 512,
	port: { min: 1, max: 65535 },
	replicas: { min: 0, max: 50 },
	maxArgs: 64,
	healthcheckPathMaxLength: 256,
	intervalSeconds: { min: 1, max: 3600 },
	timeoutSeconds: { min: 1, max: 600 },
	retries: { min: 1, max: 20 },
	startPeriodSeconds: { min: 1, max: 3600 },
	maxVolumes: 10,
	memoryMb: { min: 16, max: 1 << 20 },
	cpus: { min: 0.01, max: 256 },
} as const

/** Un spec mínimo para una app nueva: el backend completa lo mismo. */
export function newServiceSpec(image: string, port: number): AppSpec {
	return {
		image,
		port,
		replicas: 1,
		command: [],
		healthcheck: { type: 'none' },
		volumes: [],
		resources: null,
	}
}

// --- Reglas -------------------------------------------------------------------------

// La ruta del healthcheck termina dentro de un comando de shell o de JS: solo caracteres
// que no necesitan escape (sin & ; | $, comillas ni espacios).
const HEALTHCHECK_PATH = /^\/[A-Za-z0-9._~/?=%+-]*$/

// Rutas de montaje sin ":" (agregaría opciones como ":ro"), saltos de línea ni "$".
const MOUNT = /^\/[A-Za-z0-9._/-]{1,255}$/

function cleanAbsolutePath(path: string): string {
	const parts: string[] = []
	for (const part of path.split('/')) {
		if (part === '' || part === '.') continue
		if (part === '..') parts.pop()
		else parts.push(part)
	}
	return `/${parts.join('/')}`
}

/** Absoluta, limpia (`path.Clean` no la cambia) y distinta de `/`. */
export function isValidMount(mount: string): boolean {
	return MOUNT.test(mount) && cleanAbsolutePath(mount) === mount
}

export function isValidHealthcheckPath(path: string): boolean {
	return path.length <= SPEC_LIMITS.healthcheckPathMaxLength && HEALTHCHECK_PATH.test(path)
}

// La gramática de `distribution/reference` (la que usa `ParseNormalizedNamed`).
const PATH_COMPONENT = '[a-z0-9]+(?:(?:[._]|__|[-]+)[a-z0-9]+)*'
const DOMAIN_COMPONENT = '(?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9])'
const DOMAIN = `(?:${DOMAIN_COMPONENT}(?:\\.${DOMAIN_COMPONENT})*|\\[[a-fA-F0-9:]+\\])(?::[0-9]+)?`
const TAG = '[\\w][\\w.-]{0,127}'
const DIGEST = '[A-Za-z][A-Za-z0-9]*(?:[-_+.][A-Za-z][A-Za-z0-9]*)*:[0-9a-fA-F]{32,}'
const REFERENCE = new RegExp(
	`^((?:${DOMAIN}/)?${PATH_COMPONENT}(?:/${PATH_COMPONENT})*)(?::(${TAG}))?(?:@(${DIGEST}))?$`,
)
const REPOSITORY_PATH_MAX_LENGTH = 255
const DOCKER_HUB = 'docker.io'
const LEGACY_DOCKER_HUB = 'index.docker.io'
const DIGEST_HEX_LENGTH: Readonly<Record<string, number>> = { sha256: 64, sha384: 96, sha512: 128 }

function isValidDigest(digest: string): boolean {
	const [algorithm = '', hex = ''] = digest.split(':')
	const length = DIGEST_HEX_LENGTH[algorithm]
	return length !== undefined && hex.length === length && /^[a-f0-9]+$/.test(hex)
}

/**
 * Una referencia de imagen de Docker bien formada, como la acepta el backend: registro y
 * repositorio en minúsculas, tag y digest opcionales (`nats:2.10-alpine`,
 * `ghcr.io/aumi-app/backend-app:sha-3f9c2e1`, `redis@sha256:…`).
 */
export function isValidImage(image: string): boolean {
	if (image.startsWith('-') || image.includes('$')) return false
	if (/^[a-f0-9]{64}$/.test(image)) return false

	// `splitDockerDomain`: lo que va antes de la primera "/" es un registro si tiene "." o
	// ":", es "localhost" o tiene mayúsculas; si no, la imagen es de docker.io.
	// `index.docker.io` es docker.io, y una imagen de docker.io sin "/" es oficial
	// (`library/<nombre>`), con o sin el registro escrito: el límite de 255 cuenta ese
	// prefijo.
	let domain = DOCKER_HUB
	let remainder = image
	const slash = image.indexOf('/')
	const first = slash === -1 ? '' : image.slice(0, slash)
	if (
		slash !== -1 &&
		(/[.:]/.test(first) || first === 'localhost' || first !== first.toLowerCase())
	) {
		domain = first
		remainder = image.slice(slash + 1)
	}
	if (domain === LEGACY_DOCKER_HUB) domain = DOCKER_HUB
	if (domain === DOCKER_HUB && !remainder.includes('/')) remainder = `library/${remainder}`

	const tagAt = remainder.indexOf(':')
	const repository = tagAt === -1 ? remainder : remainder.slice(0, tagAt)
	if (repository !== repository.toLowerCase()) return false

	const match = REFERENCE.exec(`${domain}/${remainder}`)
	if (!match) return false
	// El límite es del path del repositorio, sin el registro (`repo.path` en Parse).
	const path = (match[1] ?? '').slice(domain.length + 1)
	if (path.length > REPOSITORY_PATH_MAX_LENGTH) return false
	return match[3] === undefined || isValidDigest(match[3])
}

// --- Schema ---------------------------------------------------------------------------

/** Los textos de cada regla. Funciones: se evalúan al validar, con el idioma de ese momento. */
export interface AppSpecMessages {
	imageRequired: () => string
	imageInvalid: () => string
	port: () => string
	replicas: () => string
	commandTooLong: () => string
	healthcheckPath: () => string
	healthcheckCommand: () => string
	interval: () => string
	timeout: () => string
	retries: () => string
	startPeriod: () => string
	volumesTooMany: () => string
	volumeName: () => string
	volumeMount: () => string
	volumeNameRepeated: (name: string) => string
	volumeMountRepeated: (mount: string) => string
	memory: () => string
	cpus: () => string
}

/** Los mensajes del backend (`appspec.go`), para cuando nadie pasa los traducidos. */
export const SPEC_MESSAGES_ES: AppSpecMessages = {
	imageRequired: () => 'es obligatoria y de hasta 512 caracteres',
	imageInvalid: () =>
		'la imagen no es una referencia válida de Docker (registro/repositorio[:tag][@sha256:…], en minúsculas)',
	port: () => 'debe estar entre 1 y 65535',
	replicas: () => 'debe estar entre 0 y 50',
	commandTooLong: () => 'máximo 64 argumentos',
	healthcheckPath: () => 'debe empezar con / y usar solo letras, números y ._~/?=%+-',
	healthcheckCommand: () => 'es obligatorio con type command (máximo 64 argumentos)',
	interval: () => 'debe estar entre 1 y 3600',
	timeout: () => 'debe estar entre 1 y 600',
	retries: () => 'debe estar entre 1 y 20',
	startPeriod: () => 'debe estar entre 1 y 3600',
	volumesTooMany: () => 'máximo 10',
	volumeName: () =>
		"no es válido: hasta 16 caracteres, solo a-z, 0-9 y '-', empieza con letra y no termina en '-'",
	volumeMount: () =>
		'debe ser una ruta absoluta limpia distinta de /, de hasta 256 caracteres, con solo letras, números y ._/-',
	volumeNameRepeated: (name) => `"${name}" está repetido`,
	volumeMountRepeated: (mount) => `"${mount}" está repetido`,
	memory: () => 'debe estar entre 16 y 1048576',
	cpus: () => 'debe estar entre 0.01 y 256',
}

// Un entero en un rango con un solo mensaje, como el backend ("debe estar entre 1 y 65535"),
// sirva para un número fuera de rango, con decimales o que falta (NaN).
function intIn(range: { min: number; max: number }, message: () => string) {
	return z
		.number({ error: message })
		.int({ error: message })
		.min(range.min, { error: message })
		.max(range.max, { error: message })
}

function argsOf(min: number, message: () => string) {
	return z
		.array(z.string())
		.min(min, { error: message })
		.max(SPEC_LIMITS.maxArgs, { error: message })
}

export function appSpecSchema(messages: AppSpecMessages = SPEC_MESSAGES_ES): z.ZodType<AppSpec> {
	const timings = {
		intervalSeconds: intIn(SPEC_LIMITS.intervalSeconds, messages.interval),
		timeoutSeconds: intIn(SPEC_LIMITS.timeoutSeconds, messages.timeout),
		retries: intIn(SPEC_LIMITS.retries, messages.retries),
		startPeriodSeconds: intIn(SPEC_LIMITS.startPeriodSeconds, messages.startPeriod),
	}
	const path = z.string().refine(isValidHealthcheckPath, { error: messages.healthcheckPath })

	return z.object({
		image: z
			.string()
			.min(1, { error: messages.imageRequired })
			.max(SPEC_LIMITS.imageMaxLength, { error: messages.imageRequired })
			.refine(isValidImage, { error: messages.imageInvalid, abort: true }),
		port: intIn(SPEC_LIMITS.port, messages.port),
		replicas: intIn(SPEC_LIMITS.replicas, messages.replicas),
		command: argsOf(0, messages.commandTooLong),
		healthcheck: z.discriminatedUnion('type', [
			z.object({ type: z.literal('none') }),
			z.object({ type: z.literal('http'), path, ...timings }),
			z.object({ type: z.literal('bun-http'), path, ...timings }),
			z.object({
				type: z.literal('command'),
				command: argsOf(1, messages.healthcheckCommand),
				...timings,
			}),
		]),
		volumes: z
			.array(
				z.object({
					name: z.string().refine(isValidVolumeName, { error: messages.volumeName }),
					mount: z.string().refine(isValidMount, { error: messages.volumeMount }),
				}),
			)
			.max(SPEC_LIMITS.maxVolumes, { error: messages.volumesTooMany })
			.superRefine((volumes, ctx) => {
				const names = new Set<string>()
				const mounts = new Set<string>()
				volumes.forEach((volume, index) => {
					if (names.has(volume.name)) {
						ctx.addIssue({
							code: 'custom',
							path: [index, 'name'],
							message: messages.volumeNameRepeated(volume.name),
						})
					}
					if (mounts.has(volume.mount)) {
						ctx.addIssue({
							code: 'custom',
							path: [index, 'mount'],
							message: messages.volumeMountRepeated(volume.mount),
						})
					}
					names.add(volume.name)
					mounts.add(volume.mount)
				})
			}),
		resources: z
			.object({
				memoryMb: intIn(SPEC_LIMITS.memoryMb, messages.memory).nullable(),
				cpus: z
					.number({ error: messages.cpus })
					.min(SPEC_LIMITS.cpus.min, { error: messages.cpus })
					.max(SPEC_LIMITS.cpus.max, { error: messages.cpus })
					.nullable(),
			})
			.nullable(),
	})
}

export interface SpecIssue {
	/** Con los nombres del dominio: `port`, `healthcheck.path`, `volumes[1].mount`. */
	readonly field: string
	readonly message: string
}

/** `['volumes', 1, 'mount']` → `volumes[1].mount`, como nombra los campos el backend. */
export function specFieldPath(path: readonly PropertyKey[]): string {
	return path.reduce<string>((field, key) => {
		if (typeof key === 'number') return `${field}[${key}]`
		return field ? `${field}.${String(key)}` : String(key)
	}, '')
}

/** Todos los problemas del spec (vacío si es válido). */
export function specIssues(spec: AppSpec, messages?: AppSpecMessages): SpecIssue[] {
	const result = appSpecSchema(messages).safeParse(spec)
	if (result.success) return []
	return result.error.issues.map((issue) => ({
		field: specFieldPath(issue.path),
		message: issue.message,
	}))
}

/** Valida el spec antes de mandarlo; el primer problema sale como `InvalidAppSpec`. */
export function parseAppSpec(spec: AppSpec): AppSpec {
	const [first] = specIssues(spec)
	if (first) throw new InvalidAppSpec(first.field, first.message)
	return spec
}

// --- Comparación y vista previa --------------------------------------------------------

function canonical(value: unknown): unknown {
	if (Array.isArray(value)) return value.map(canonical)
	if (value && typeof value === 'object') {
		return Object.fromEntries(
			Object.entries(value)
				.sort(([a], [b]) => a.localeCompare(b))
				.map(([key, item]) => [key, canonical(item)]),
		)
	}
	return value
}

/** Mismo contenido, sin importar el orden de las claves. */
export function specsEqual(a: AppSpec, b: AppSpec): boolean {
	return JSON.stringify(canonical(a)) === JSON.stringify(canonical(b))
}

function execForm(args: readonly string[]): string {
	return `[${args.map((arg) => JSON.stringify(arg)).join(', ')}]`
}

/**
 * El healthcheck como lo va a generar la Fase 2 (§6.1): `http` en forma exec con wget,
 * `bun-http` con un fetch de bun y `command` con sus argumentos. `null` para `none`.
 */
export function healthcheckPreview(healthcheck: Healthcheck, port: number): string | null {
	switch (healthcheck.type) {
		case 'none':
			return null
		case 'http':
			return execForm([
				'CMD',
				'wget',
				'-q',
				'-O',
				'/dev/null',
				`http://127.0.0.1:${port}${healthcheck.path}`,
			])
		case 'bun-http':
			return `CMD bun -e "fetch('http://127.0.0.1:${port}${healthcheck.path}').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"`
		case 'command':
			return execForm(['CMD', ...healthcheck.command])
	}
}
