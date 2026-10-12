import type { CatalogDeps } from '../repositories/catalogRepository'
import type { ProjectSlug, StageSlug } from '../value-objects/slugs'

export interface DeleteStageInput {
	project: ProjectSlug
	stage: StageSlug
}

/** `StageNotEmpty` mientras tenga apps. Sus variables se borran con él. */
export type DeleteStage = (input: DeleteStageInput) => Promise<void>

export function makeDeleteStage({ catalog }: CatalogDeps): DeleteStage {
	return async ({ project, stage }) => catalog.deleteStage(project, stage)
}
