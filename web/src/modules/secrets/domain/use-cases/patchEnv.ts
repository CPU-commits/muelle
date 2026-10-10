import type { EnvOwner } from '../entities/envOwner'
import { assertPatchWithinLimits, type EnvPatchInput, parseEnvPatch } from '../entities/envPatch'
import type { EnvVar } from '../entities/envVar'
import type { SecretsDeps } from '../repositories/envRepository'

export interface PatchEnvInput extends EnvPatchInput {
	readonly owner: EnvOwner
	/**
	 * Lo que el panel ya tiene del dueño. Con esto se revisan los límites del dueño
	 * (500 variables, 1 MiB) antes de mandar; sin esto, los revisa solo el backend.
	 */
	readonly current?: readonly EnvVar[]
}

/**
 * Agrega o reemplaza las variables de `set` (todas con el mismo `secret`) y borra las de
 * `unset`. Deja con cambios pendientes a la app, o a todas las apps del stage.
 */
export type PatchEnv = (input: PatchEnvInput) => Promise<void>

export function makePatchEnv({ env }: SecretsDeps): PatchEnv {
	return async ({ owner, current, ...input }) => {
		const patch = parseEnvPatch(input)
		if (current) assertPatchWithinLimits(current, patch)
		await env.patch(owner, patch)
	}
}
