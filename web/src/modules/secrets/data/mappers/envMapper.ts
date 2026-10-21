import type { EnvKey, EnvPatch, EnvReplacement, EnvVar } from '../../domain'
import type { EnvDto, EnvVarDto, PatchEnvPayload, ReplaceEnvPayload } from '../dtos/envDto'

// La clave que devuelve el backend ya pasó su validación: se marca como EnvKey sin volver
// a aplicar la regla (si la regla cambia allá, el panel no deja de mostrar las variables).
function asEnvKey(raw: string): EnvKey {
	return raw as EnvKey
}

export function envVarFromDto(dto: EnvVarDto): EnvVar {
	const key = asEnvKey(dto.key)
	const updatedAt = new Date(dto.updated_at)
	// El schema ya garantiza que `value` es null si y solo si es secreta.
	return dto.value === null
		? { key, secret: true, value: null, updatedAt }
		: { key, secret: false, value: dto.value, updatedAt }
}

export function envFromDto(dto: EnvDto): EnvVar[] {
	return dto.vars.map(envVarFromDto)
}

// `Object.fromEntries` define cada clave como propiedad propia: `__proto__` es una clave
// válida y con `obj[key] = v` cambiaría el prototipo en vez de viajar en el JSON.
function toRecord(entries: ReadonlyMap<EnvKey, string>): Record<string, string> {
	return Object.fromEntries(entries)
}

/** Sin `set` ni `secret` si no hay nada que agregar; sin `unset` si no hay nada que borrar. */
export function patchPayload(patch: EnvPatch): PatchEnvPayload {
	return {
		...(patch.set.size > 0 ? { set: toRecord(patch.set), secret: patch.secret } : {}),
		...(patch.unset.length > 0 ? { unset: [...patch.unset] } : {}),
	}
}

export function replacePayload(replacement: EnvReplacement): ReplaceEnvPayload {
	return { vars: toRecord(replacement.vars), secret: replacement.secret }
}
