import type { Registry } from '../entities/registry'
import type { RegistryDeps } from '../repositories/adminRepositories'
import { parseRegistryCredentials } from '../value-objects/registryCredentials'
import { parseRegistryHost } from '../value-objects/registryHost'

export interface RegistryInput {
	host: string
	username: string
	password: string
}

/** Un host repetido responde `RegistryExists`. */
export type AddRegistry = (input: RegistryInput) => Promise<Registry>

export function makeAddRegistry({ registries }: RegistryDeps): AddRegistry {
	return async (input) =>
		registries.create({
			host: parseRegistryHost(input.host),
			...parseRegistryCredentials(input),
		})
}
