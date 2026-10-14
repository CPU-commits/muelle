import type { Registry } from '../entities/registry'
import type { RegistryDeps } from '../repositories/adminRepositories'
import { parseRegistryCredentials } from '../value-objects/registryCredentials'
import type { RegistryInput } from './addRegistry'

/** Reemplaza usuario y contraseña de un registry que ya existe; el host no cambia. */
export type UpdateRegistry = (input: RegistryInput) => Promise<Registry>

export function makeUpdateRegistry({ registries }: RegistryDeps): UpdateRegistry {
	return async (input) => registries.update(input.host, parseRegistryCredentials(input))
}
