import type { Project } from '../entities/project'
import type { CatalogDeps } from '../repositories/catalogRepository'
import { parseProjectName } from '../value-objects/projectName'
import { parseProjectSlug } from '../value-objects/slugs'

export interface CreateProjectInput {
	slug: string
	name: string
}

/** `SlugTaken` si el slug ya existe. */
export type CreateProject = (input: CreateProjectInput) => Promise<Project>

export function makeCreateProject({ catalog }: CatalogDeps): CreateProject {
	return async ({ slug, name }) =>
		catalog.createProject({ slug: parseProjectSlug(slug), name: parseProjectName(name) })
}
