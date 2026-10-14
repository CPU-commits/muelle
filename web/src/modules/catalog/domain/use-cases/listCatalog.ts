import type { CatalogTree } from '../entities/catalogTree'
import type { CatalogDeps } from '../repositories/catalogRepository'

/** Todos los proyectos con sus stages y sus apps (Proyectos y los selectores de una app). */
export type ListCatalog = (signal?: AbortSignal) => Promise<CatalogTree>

// La API no tiene un endpoint que devuelva el árbol: se arma con un pedido por proyecto y
// uno por stage, en paralelo. En un muelle de una persona son unas pocas decenas.
export function makeListCatalog({ catalog }: CatalogDeps): ListCatalog {
	return async (signal) => {
		const projects = await catalog.listProjects(signal)
		return Promise.all(
			projects.map(async (project) => {
				const stages = await catalog.listStages(project.slug, signal)
				return {
					project,
					stages: await Promise.all(
						stages.map(async (stage) => ({
							stage,
							apps: await catalog.listApps(project.slug, stage.slug, signal),
						})),
					),
				}
			}),
		)
	}
}
