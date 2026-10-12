import type { ProjectSlug } from '../value-objects/slugs'

/** Agrupa los stages de un producto (`auditfy`). El slug es único en el panel. */
export interface Project {
	readonly slug: ProjectSlug
	/** El nombre visible ("Auditfy"); se puede cambiar, el slug no. */
	readonly name: string
	readonly createdAt: Date
}

/** La letra del cuadradito que acompaña al proyecto. */
export function projectInitial(project: Pick<Project, 'slug'>): string {
	return project.slug.charAt(0)
}
