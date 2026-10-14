import type { EnvOwner } from '../entities/envOwner'
import { type EnvReplacementInput, parseEnvReplacement } from '../entities/envPatch'
import type { SecretsDeps } from '../repositories/envRepository'

export interface ReplaceEnvInput extends EnvReplacementInput {
	readonly owner: EnvOwner
}

/**
 * Reemplaza TODO el set del dueño (PUT, como el `sync-env` de GitHub Actions). Deja con
 * cambios pendientes a la app, o a todas las apps del stage.
 */
export type ReplaceEnv = (input: ReplaceEnvInput) => Promise<void>

export function makeReplaceEnv({ env }: SecretsDeps): ReplaceEnv {
	return async ({ owner, ...input }) => env.replace(owner, parseEnvReplacement(input))
}
