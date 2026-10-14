import type { EnvOwner } from '../entities/envOwner'
import type { EnvPatch, EnvReplacement } from '../entities/envPatch'
import type { EnvVar } from '../entities/envVar'

// El puerto de las variables de un stage o una app. La implementación HTTP traduce los
// errores del backend a los de `errors/secretsErrors.ts` (y un 422 a `ValidationError`
// con su campo: `vars`, `set` o `unset`); ningún método devuelve un status.
export interface EnvRepository {
	/** Las variables del dueño, con las secretas enmascaradas (`value: null`). */
	list(owner: EnvOwner, signal?: AbortSignal): Promise<EnvVar[]>
	/** Reemplaza todo el set (PUT). */
	replace(owner: EnvOwner, replacement: EnvReplacement): Promise<void>
	/** Agrega, reemplaza y borra (PATCH). */
	patch(owner: EnvOwner, patch: EnvPatch): Promise<void>
}

export interface SecretsDeps {
	readonly env: EnvRepository
}
