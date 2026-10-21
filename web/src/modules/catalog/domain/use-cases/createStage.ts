import type { Stage } from '../entities/stage'
import type { CatalogDeps } from '../repositories/catalogRepository'
import { type ProjectSlug, parseStageSlug } from '../value-objects/slugs'

export interface CreateStageInput {
	project: ProjectSlug
	slug: string
}

/** `SlugTaken` si el proyecto ya tiene un stage con ese slug. */
export type CreateStage = (input: CreateStageInput) => Promise<Stage>

export function makeCreateStage({ catalog }: CatalogDeps): CreateStage {
	return async ({ project, slug }) => catalog.createStage(project, parseStageSlug(slug))
}
