import { http, HttpResponse, type RequestHandler } from 'msw'
import { api, apiError, invalid, noContent, notFound } from '../api'
import { catalogDb } from '../db/catalog'
import { type MockEnvTarget, type MockEnvVar, secretsDb } from '../db/secrets'
import { requireSession } from '../session'

// `GET/PUT/PATCH …/stages/{s}/env` y `…/apps/{a}/env` de secrets (infra/in/http/env.go),
// con las reglas de `entity`, `validate.EnvKey` y los casos de uso: los secretos vuelven
// enmascarados, un PATCH no reescribe lo que no cambia, y los límites (500 variables,
// 64 KiB por valor, 1 MiB por dueño y por cuerpo) responden lo mismo que el backend.

// Los dueños viven en el mock de catalog: un stage o una app que no está ahí es 404, y
// cambiar sus variables deja con cambios pendientes a la app o a todas las apps del stage
// (§5.2 regla 4 del diseño). Un test puede cambiar cualquiera de los dos (`null` vuelve a
// "todo existe" y "nadie se entera").
secretsDb.setOwnerResolver(({ project, stage, app }) =>
	app === undefined
		? catalogDb.findStage(project, stage) !== undefined
		: catalogDb.findApp(project, stage, app) !== undefined,
)
secretsDb.setChangeListener(({ project, stage, app }) => catalogDb.touchConfig(project, stage, app))

const MAX_VARS = 500
const MAX_VALUE_BYTES = 64 * 1024
const MAX_OWNER_BYTES = 1024 * 1024
const MAX_BODY_BYTES = 1024 * 1024
const ENV_KEY = /^[A-Za-z_][A-Za-z0-9_]{0,127}$/

type Field = 'vars' | 'set' | 'unset'

const bytes = (text: string) => new TextEncoder().encode(text).length
const quote = (text: string) => JSON.stringify(text)

/** `ORDER BY key` de SQLite: binario. */
const byKey = (a: MockEnvVar, b: MockEnvVar) => (a.key < b.key ? -1 : a.key > b.key ? 1 : 0)

function masked(envVar: MockEnvVar) {
	return {
		key: envVar.key,
		value: envVar.secret ? null : envVar.value,
		secret: envVar.secret,
		updated_at: envVar.updatedAt,
	}
}

function badJson(message: string) {
	return apiError(400, 'invalid_json', message)
}

// `utils.DecodeJSON`: cuerpo de hasta 1 MiB, un objeto, sin campos desconocidos ni
// UTF-8 inválido, con los tipos de cada campo.
async function decode(
	request: Request,
	fields: Record<string, 'record' | 'list' | 'bool'>,
): Promise<{ body: Record<string, unknown> } | { error: Response }> {
	const raw = await request.text()
	if (bytes(raw) > MAX_BODY_BYTES) return { error: badJson('el cuerpo supera 1 MiB') }
	const trimmed = raw.trimStart()
	if (trimmed.length > 0 && !trimmed.startsWith('{')) {
		return { error: badJson('JSON inválido: el cuerpo debe ser un objeto') }
	}
	let body: Record<string, unknown>
	try {
		body = JSON.parse(raw) as Record<string, unknown>
	} catch {
		return { error: badJson('JSON inválido: el cuerpo está vacío o incompleto') }
	}
	for (const [name, value] of Object.entries(body)) {
		// `hasOwn`: `constructor` o `__proto__` no son campos del cuerpo (el backend da 400).
		const kind = Object.hasOwn(fields, name) ? fields[name] : undefined
		if (!kind) return { error: badJson(`JSON inválido: el campo ${quote(name)} no existe`) }
		if (value === null) continue
		const wrongType = badJson(
			`JSON inválido: el campo ${quote(name)} tiene un tipo de valor incorrecto`,
		)
		if (kind === 'bool' && typeof value !== 'boolean') return { error: wrongType }
		if (kind === 'list') {
			if (!Array.isArray(value) || value.some((item) => typeof item !== 'string')) {
				return { error: wrongType }
			}
			if (value.some((item: string) => !item.isWellFormed()))
				return { error: badJson('JSON inválido') }
		}
		if (kind === 'record') {
			if (typeof value !== 'object' || Array.isArray(value)) return { error: wrongType }
			for (const [key, item] of Object.entries(value)) {
				if (typeof item !== 'string') return { error: wrongType }
				if (!key.isWellFormed() || !item.isWellFormed())
					return { error: badJson('JSON inválido') }
			}
		}
	}
	return { body }
}

// `validate.EnvKey`.
function keyError(field: Field, key: string) {
	if (!ENV_KEY.test(key)) {
		return invalid(
			field,
			`clave ${quote(key)} inválida: letras, números y '_', sin empezar con número, hasta 128`,
		)
	}
	if (key.startsWith('COMPOSE_')) {
		return invalid(field, `clave ${quote(key)} inválida: el prefijo COMPOSE_ está reservado`)
	}
	return undefined
}

// `entity.NewEnvVars`: en orden de clave, la clave y después el valor.
function varsError(field: Field, vars: Record<string, string>) {
	for (const key of Object.keys(vars).sort()) {
		const value = vars[key] ?? ''
		const error =
			keyError(field, key) ??
			(bytes(value) > MAX_VALUE_BYTES
				? invalid(field, `el valor de ${quote(key)} supera 64 KiB`)
				: undefined) ??
			(value.includes('\0')
				? invalid(field, `el valor de ${quote(key)} no puede contener el carácter NUL`)
				: undefined)
		if (error) return error
	}
	return undefined
}

function totalsError(field: Field, vars: readonly MockEnvVar[]) {
	if (vars.length > MAX_VARS) return invalid(field, `máximo ${MAX_VARS} variables`)
	const size = vars.reduce((total, envVar) => total + bytes(envVar.key) + bytes(envVar.value), 0)
	if (size > MAX_OWNER_BYTES) {
		return invalid(field, 'las variables suman más de 1 MiB (claves y valores)')
	}
	return undefined
}

function targetOf(params: Record<string, string | readonly string[] | undefined>): MockEnvTarget {
	const text = (name: string) => String(params[name] ?? '')
	const app = text('a')
	return { project: text('p'), stage: text('s'), ...(app ? { app } : {}) }
}

function getEnv(target: MockEnvTarget) {
	if (!secretsDb.ownerExists(target)) return notFound()
	return HttpResponse.json({ vars: [...secretsDb.vars(target)].sort(byKey).map(masked) })
}

async function putEnv(request: Request, target: MockEnvTarget) {
	const decoded = await decode(request, { vars: 'record', secret: 'bool' })
	if ('error' in decoded) return decoded.error
	const { vars, secret } = decoded.body as {
		vars?: Record<string, string> | null
		secret?: boolean | null
	}
	if (!vars) return invalid('vars', 'es obligatorio')
	if (Object.keys(vars).length > MAX_VARS) return invalid('vars', `máximo ${MAX_VARS} variables`)
	const error = varsError('vars', vars)
	if (error) return error
	const at = new Date().toISOString()
	const next = Object.entries(vars).map(([key, value]) => ({
		key,
		value,
		secret: secret ?? true,
		updatedAt: at,
	}))
	const limits = totalsError('vars', next)
	if (limits) return limits
	if (!secretsDb.ownerExists(target)) return notFound()
	secretsDb.save(target, next)
	return noContent()
}

async function patchEnv(request: Request, target: MockEnvTarget) {
	const decoded = await decode(request, { set: 'record', unset: 'list', secret: 'bool' })
	if ('error' in decoded) return decoded.error
	const body = decoded.body as {
		set?: Record<string, string> | null
		unset?: string[] | null
		secret?: boolean | null
	}
	const set = body.set ?? {}
	const unset = body.unset ?? []
	const secret = body.secret ?? true
	if (Object.keys(set).length === 0 && unset.length === 0) {
		return invalid('set', 'indica al menos una variable en set o unset')
	}
	if (unset.length > MAX_VARS) return invalid('unset', `máximo ${MAX_VARS} claves`)
	if (Object.keys(set).length > MAX_VARS) return invalid('set', `máximo ${MAX_VARS} variables`)
	for (const key of unset) {
		const error = keyError('unset', key)
		if (error) return error
		if (Object.hasOwn(set, key)) {
			return invalid('unset', `${quote(key)} no puede estar en set y unset a la vez`)
		}
	}
	const error = varsError('set', set)
	if (error) return error
	if (!secretsDb.ownerExists(target)) return notFound()

	const current = new Map(secretsDb.vars(target).map((envVar) => [envVar.key, envVar]))
	// Lo que tiene el mismo valor y el mismo secret no es un cambio y no se reescribe.
	const changed = Object.entries(set).filter(([key, value]) => {
		const stored = current.get(key)
		return !stored || stored.value !== value || stored.secret !== secret
	})
	if (changed.length === 0 && !unset.some((key) => current.has(key))) return noContent()

	for (const key of unset) current.delete(key)
	const at = new Date().toISOString()
	for (const [key, value] of changed) current.set(key, { key, value, secret, updatedAt: at })
	const next = [...current.values()]
	// Solo se pueden superar los límites agregando: borrar siempre se permite.
	if (changed.length > 0) {
		const limits = totalsError('set', next)
		if (limits) return limits
	}
	secretsDb.save(target, next)
	return noContent()
}

const PATHS = ['/projects/:p/stages/:s/env', '/projects/:p/stages/:s/apps/:a/env']

export const secretsHandlers: RequestHandler[] = PATHS.flatMap((path) => [
	http.get(api(path), ({ params }) => requireSession() ?? getEnv(targetOf(params))),
	http.put(
		api(path),
		({ request, params }) => requireSession() ?? putEnv(request, targetOf(params)),
	),
	http.patch(
		api(path),
		({ request, params }) => requireSession() ?? patchEnv(request, targetOf(params)),
	),
])
