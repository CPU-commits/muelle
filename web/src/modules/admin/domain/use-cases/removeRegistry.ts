import type { RegistryDeps } from '../repositories/adminRepositories'

/** Las apps que usen imágenes de ese host ya no van a poder bajarlas. */
export type RemoveRegistry = (host: string) => Promise<void>

export function makeRemoveRegistry({ registries }: RegistryDeps): RemoveRegistry {
	return async (host) => registries.remove(host)
}
