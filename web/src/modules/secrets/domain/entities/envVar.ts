import type { EnvKey } from '../value-objects/envKey'
import { envVarSize, utf8Length } from '../value-objects/envLimits'

/**
 * Una variable tal como la devuelve la API. El valor de una secreta nunca vuelve: no se
 * puede leer, solo reemplazar (`GET` responde `"value": null`).
 */
export type EnvVar =
	| {
			readonly key: EnvKey
			readonly secret: false
			readonly value: string
			readonly updatedAt: Date
	  }
	| {
			readonly key: EnvKey
			readonly secret: true
			readonly value: null
			readonly updatedAt: Date
	  }

/** El orden del backend (`ORDER BY key` de SQLite, binario): mayúsculas antes que `_`. */
export function compareEnvKeys(a: string, b: string): number {
	if (a === b) return 0
	return a < b ? -1 : 1
}

export function sortEnvVars(vars: readonly EnvVar[]): EnvVar[] {
	return [...vars].sort((a, b) => compareEnvKeys(a.key, b.key))
}

export function findEnvVar(vars: readonly EnvVar[], key: string): EnvVar | undefined {
	return vars.find((envVar) => envVar.key === key)
}

export function envKeys(vars: readonly EnvVar[]): Set<string> {
	return new Set(vars.map((envVar) => envVar.key))
}

/**
 * Lo mínimo que ocupan las variables (claves + valores): de una secreta solo se conoce
 * la clave. El backend mide lo real; esto alcanza para avisar antes de mandar.
 */
export function knownEnvSize(vars: readonly EnvVar[]): number {
	let size = 0
	for (const envVar of vars) {
		size += envVar.secret ? utf8Length(envVar.key) : envVarSize(envVar.key, envVar.value)
	}
	return size
}

/** Las claves de `vars` que también están en `other` (las que la app pisa del stage). */
export function overlappingKeys(vars: readonly EnvVar[], other: readonly EnvVar[]): Set<string> {
	const keys = envKeys(other)
	return new Set(vars.filter((envVar) => keys.has(envVar.key)).map((envVar) => envVar.key))
}

/**
 * El buscador de la lista: por clave, sin distinguir mayúsculas, y por el valor de las que
 * no son secretas.
 */
export function filterEnvVars(vars: readonly EnvVar[], query: string): readonly EnvVar[] {
	const needle = query.trim().toLowerCase()
	if (needle.length === 0) return vars
	return vars.filter(
		(envVar) =>
			envVar.key.toLowerCase().includes(needle) ||
			(!envVar.secret && envVar.value.toLowerCase().includes(needle)),
	)
}
