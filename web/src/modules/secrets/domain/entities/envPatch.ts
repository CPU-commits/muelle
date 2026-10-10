import {
	EmptyEnvPatch,
	type EnvField,
	EnvKeyInSetAndUnset,
	type EnvLimit,
	EnvLimitExceeded,
	InvalidEnvValue,
} from '../errors/secretsErrors'
import { type EnvKey, parseEnvKey } from '../value-objects/envKey'
import {
	envValueProblem,
	envVarSize,
	MAX_ENV_VARS,
	MAX_OWNER_BYTES,
	utf8Length,
} from '../value-objects/envLimits'
import { compareEnvKeys, type EnvVar } from './envVar'

/**
 * Un PATCH de variables: agrega o reemplaza las de `set` y borra las de `unset`. El
 * backend aplica un solo `secret` a todas las de `set`: para mezclar secretas y no
 * secretas hacen falta dos PATCH.
 */
export interface EnvPatch {
	/** Clave → valor. Un `Map`, no un objeto: `__proto__` es una clave válida. */
	readonly set: ReadonlyMap<EnvKey, string>
	/** Ordenadas y sin repetidos. */
	readonly unset: readonly EnvKey[]
	readonly secret: boolean
}

/** Un PUT: reemplaza TODO el set del dueño, con un `secret` para todas. */
export interface EnvReplacement {
	readonly vars: ReadonlyMap<EnvKey, string>
	readonly secret: boolean
}

/** Pares clave → valor sin validar (de un formulario, de un `.env`). */
export type RawEnvEntries = Iterable<readonly [string, string]>

export interface EnvPatchInput {
	readonly set?: RawEnvEntries
	readonly unset?: Iterable<string>
	readonly secret: boolean
}

export interface EnvReplacementInput {
	readonly vars: RawEnvEntries
	readonly secret: boolean
}

function validEntries(entries: Map<string, string>, field: EnvField): Map<EnvKey, string> {
	const out = new Map<EnvKey, string>()
	// En orden de clave, como `NewEnvVars`: el primer error es el mismo que daría el backend.
	for (const key of [...entries.keys()].sort(compareEnvKeys)) {
		const value = entries.get(key) ?? ''
		const envKey = parseEnvKey(key, field)
		const problem = envValueProblem(value)
		if (problem) throw new InvalidEnvValue(problem, key, field)
		out.set(envKey, value)
	}
	return out
}

/**
 * Valida un PATCH con las reglas de `PatchEnv.Execute`, en su mismo orden: que pida algo,
 * los tamaños de `unset` y `set`, las claves de `unset` (y que no estén también en `set`),
 * y después cada variable de `set`.
 */
export function parseEnvPatch(input: EnvPatchInput): EnvPatch {
	const set = new Map(input.set ?? [])
	const unset = [...(input.unset ?? [])]
	if (set.size === 0 && unset.length === 0) throw new EmptyEnvPatch()
	if (unset.length > MAX_ENV_VARS) throw new EnvLimitExceeded('count', 'unset')
	if (set.size > MAX_ENV_VARS) throw new EnvLimitExceeded('count', 'set')
	const unsetKeys = unset.map((key) => {
		const envKey = parseEnvKey(key, 'unset')
		if (set.has(key)) throw new EnvKeyInSetAndUnset(key)
		return envKey
	})
	return {
		set: validEntries(set, 'set'),
		unset: [...new Set(unsetKeys)].sort(compareEnvKeys),
		secret: input.secret,
	}
}

/** Valida un PUT con las reglas de `ReplaceEnv.Execute`: cantidad, cada variable y el total. */
export function parseEnvReplacement(input: EnvReplacementInput): EnvReplacement {
	const entries = new Map(input.vars)
	if (entries.size > MAX_ENV_VARS) throw new EnvLimitExceeded('count', 'vars')
	const vars = validEntries(entries, 'vars')
	let size = 0
	for (const [key, value] of vars) size += envVarSize(key, value)
	if (size > MAX_OWNER_BYTES) throw new EnvLimitExceeded('size', 'vars')
	return { vars, secret: input.secret }
}

/**
 * Si el dueño quedaría fuera de los límites después del PATCH, con lo que el panel sabe
 * de él (`current`). De una secreta solo se conoce la clave, así que el tamaño es un
 * mínimo: si esto pasa, el backend seguro lo rechaza; si no, decide él. Como en el
 * backend, borrar siempre se permite.
 */
export function patchLimitProblem(current: readonly EnvVar[], patch: EnvPatch): EnvLimit | null {
	if (patch.set.size === 0) return null
	const sizes = new Map<string, number>()
	for (const envVar of current) {
		sizes.set(
			envVar.key,
			envVar.secret ? utf8Length(envVar.key) : envVarSize(envVar.key, envVar.value),
		)
	}
	for (const key of patch.unset) sizes.delete(key)
	for (const [key, value] of patch.set) sizes.set(key, envVarSize(key, value))
	if (sizes.size > MAX_ENV_VARS) return 'count'
	let total = 0
	for (const size of sizes.values()) total += size
	return total > MAX_OWNER_BYTES ? 'size' : null
}

/** `patchLimitProblem` que tira el error, para los casos de uso. */
export function assertPatchWithinLimits(current: readonly EnvVar[], patch: EnvPatch): void {
	const limit = patchLimitProblem(current, patch)
	if (limit) throw new EnvLimitExceeded(limit, 'set')
}
