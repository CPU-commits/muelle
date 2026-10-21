import type { Project } from '../entities/project'
import type { CatalogDeps } from '../repositories/catalogRepository'
import { parseProjectName } from '../value-objects/projectName'
import type { ProjectSlug } from '../value-objects/slugs'

export interface RenameProjectInput {
	project: ProjectSlug
	name: string
}

/** Cambia el nombre visible; el slug (y todo lo que se llama como él) queda igual. */
export type RenameProject = (input: RenameProjectInput) => Promise<Project>

export function makeRenameProject({ catalog }: CatalogDeps): RenameProject {
	return async ({ project, name }) => catalog.renameProject(project, parseProjectName(name))
}
