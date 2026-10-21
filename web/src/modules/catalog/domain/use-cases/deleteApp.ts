import type { AppRef } from '../entities/app'
import type { CatalogDeps } from '../repositories/catalogRepository'

/** Borra la app y sus variables. En la Fase 2 también baja su stack de Swarm. */
export type DeleteApp = (ref: AppRef) => Promise<void>

export function makeDeleteApp({ catalog }: CatalogDeps): DeleteApp {
	return async (ref) => catalog.deleteApp(ref)
}
