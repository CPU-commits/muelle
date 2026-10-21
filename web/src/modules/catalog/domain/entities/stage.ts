import type { ProjectSlug, StageSlug } from '../value-objects/slugs'

/** Un entorno de un proyecto (`staging`, `production`). El slug es único en su proyecto. */
export interface Stage {
	readonly project: ProjectSlug
	readonly slug: StageSlug
	readonly createdAt: Date
}

/** La red overlay del stage, `<proyecto>_<stage>` (§5.2 del spec). */
export function stageNetwork(stage: { readonly project: string; readonly slug: string }): string {
	return `${stage.project}_${stage.slug}`
}
