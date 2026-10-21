import type { App, AppRef } from '../entities/app'
import type { CatalogDeps } from '../repositories/catalogRepository'

/** `AppNotFound` si la app (o su stage o su proyecto) no existe. */
export type GetApp = (ref: AppRef, signal?: AbortSignal) => Promise<App>

export function makeGetApp({ catalog }: CatalogDeps): GetApp {
	return async (ref, signal) => catalog.getApp(ref, signal)
}
