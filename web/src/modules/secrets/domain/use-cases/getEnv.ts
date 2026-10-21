import type { EnvOwner } from '../entities/envOwner'
import { type EnvVar, sortEnvVars } from '../entities/envVar'
import type { SecretsDeps } from '../repositories/envRepository'

export type GetEnv = (owner: EnvOwner, signal?: AbortSignal) => Promise<EnvVar[]>

/** Las variables de un stage o una app, ordenadas por clave como las lista el backend. */
export function makeGetEnv({ env }: SecretsDeps): GetEnv {
	return async (owner, signal) => sortEnvVars(await env.list(owner, signal))
}
