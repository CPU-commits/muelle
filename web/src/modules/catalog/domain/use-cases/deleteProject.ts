import type { CatalogDeps } from '../repositories/catalogRepository'
import type { ProjectSlug } from '../value-objects/slugs'

/** `ProjectNotEmpty` mientras tenga stages: el backend no borra en cascada. */
export type DeleteProject = (project: ProjectSlug) => Promise<void>

export function makeDeleteProject({ catalog }: CatalogDeps): DeleteProject {
	return async (project) => catalog.deleteProject(project)
}
