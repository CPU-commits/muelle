import type { EnvPatch } from '../entities/envPatch'
import { compareEnvKeys, type EnvVar } from '../entities/envVar'
import type { EnvKey } from '../value-objects/envKey'

/**
 * - `merge`: agrega y reemplaza las del archivo; las demás quedan como están.
 * - `replace`: el dueño queda exactamente con las del archivo; las demás se borran.
 */
export type ImportMode = 'merge' | 'replace'

/** Por qué cambia una variable que ya existía. */
export type ImportChangeReason = 'value' | 'to_secret' | 'to_plain'

export interface ImportChange {
	readonly key: EnvKey
	readonly reason: ImportChangeReason
}

export interface ImportOptions {
	readonly mode: ImportMode
	/** El `secret` del PATCH: aplica a todas las del archivo. */
	readonly secret: boolean
}

export interface ImportPlan {
	/** No existían. */
	readonly added: readonly EnvKey[]
	/** Existían y cambian: el valor (si se podía leer) o la marca de secreta. */
	readonly changed: readonly ImportChange[]
	/**
	 * Existían como secretas y se importan como secretas: el valor guardado no se puede
	 * leer para compararlo, así que se mandan igual. Si es el mismo, el backend no
	 * cambia nada.
	 */
	readonly overwritten: readonly EnvKey[]
	/** Existían con el mismo valor y la misma marca: no se mandan. */
	readonly unchanged: readonly EnvKey[]
	/** Solo con `replace`: existían y no están en el archivo. */
	readonly removed: readonly EnvKey[]
	/** El PATCH que aplica el plan, o null si no hay nada que cambiar. */
	readonly patch: EnvPatch | null
}

export interface ImportEntry {
	readonly key: EnvKey
	readonly value: string
}

function changeReason(current: EnvVar, value: string, secret: boolean): ImportChangeReason | null {
	if (current.secret) return secret ? null : 'to_plain'
	if (secret) return 'to_secret'
	return current.value === value ? null : 'value'
}

/** Compara lo que trae el `.env` con lo que tiene el dueño y arma el PATCH. */
export function planImport(
	current: readonly EnvVar[],
	entries: readonly ImportEntry[],
	options: ImportOptions,
): ImportPlan {
	const existing = new Map(current.map((envVar) => [envVar.key as string, envVar]))
	const incoming = new Set(entries.map((entry) => entry.key as string))
	const set = new Map<EnvKey, string>()
	const added: EnvKey[] = []
	const changed: ImportChange[] = []
	const overwritten: EnvKey[] = []
	const unchanged: EnvKey[] = []

	for (const entry of [...entries].sort((a, b) => compareEnvKeys(a.key, b.key))) {
		const envVar = existing.get(entry.key)
		if (!envVar) {
			added.push(entry.key)
			set.set(entry.key, entry.value)
			continue
		}
		const reason = changeReason(envVar, entry.value, options.secret)
		if (reason) {
			changed.push({ key: entry.key, reason })
			set.set(entry.key, entry.value)
		} else if (envVar.secret) {
			overwritten.push(entry.key)
			set.set(entry.key, entry.value)
		} else {
			unchanged.push(entry.key)
		}
	}

	const removed =
		options.mode === 'replace'
			? current
					.filter((envVar) => !incoming.has(envVar.key))
					.map((envVar) => envVar.key)
					.sort(compareEnvKeys)
			: []

	const patch: EnvPatch | null =
		set.size > 0 || removed.length > 0 ? { set, unset: removed, secret: options.secret } : null

	return { added, changed, overwritten, unchanged, removed, patch }
}
