import type { Registry } from '../entities/registry'
import type { RegistryDeps } from '../repositories/adminRepositories'

export type ListRegistries = (signal?: AbortSignal) => Promise<Registry[]>

export function makeListRegistries({ registries }: RegistryDeps): ListRegistries {
	return async (signal) => registries.list(signal)
}
