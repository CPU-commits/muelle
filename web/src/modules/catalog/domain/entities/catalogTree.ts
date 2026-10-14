import type { ProjectSlug, StageSlug } from '../value-objects/slugs'
import { type App, type AppRef, appContents, appRefOf } from './app'
import type { Project } from './project'
import type { Stage } from './stage'

// Proyectos → stages → apps: lo que muestra Proyectos y lo que ofrecen los selectores del
// breadcrumb de una app.

export interface StageNode {
	readonly stage: Stage
	readonly apps: readonly App[]
}

export interface ProjectNode {
	readonly project: Project
	readonly stages: readonly StageNode[]
}

export type CatalogTree = readonly ProjectNode[]

export function countApps(nodes: CatalogTree | ProjectNode): number {
	const projects = Array.isArray(nodes) ? (nodes as CatalogTree) : [nodes as ProjectNode]
	return projects.reduce(
		(total, project) =>
			total + project.stages.reduce((sum, stage) => sum + stage.apps.length, 0),
		0,
	)
}

function matches(text: string, query: string): boolean {
	return text.toLowerCase().includes(query)
}

function appMatches(app: App, query: string): boolean {
	if (matches(app.slug, query)) return true
	const contents = appContents(app)
	return contents.kind === 'image'
		? matches(contents.image, query)
		: contents.services.some((service) => matches(service, query))
}

/**
 * El buscador de Proyectos. Un proyecto o un stage que coincide se muestra entero; si no,
 * solo las apps que coinciden (por slug, imagen o servicio del compose), y desaparece lo
 * que queda vacío.
 */
export function filterCatalog(tree: CatalogTree, rawQuery: string): CatalogTree {
	const query = rawQuery.trim().toLowerCase()
	if (query === '') return tree

	return tree.flatMap((node) => {
		if (matches(node.project.slug, query) || matches(node.project.name, query)) return [node]
		const stages = node.stages.flatMap((stageNode) => {
			if (matches(stageNode.stage.slug, query)) return [stageNode]
			const apps = stageNode.apps.filter((app) => appMatches(app, query))
			return apps.length > 0 ? [{ ...stageNode, apps }] : []
		})
		return stages.length > 0 ? [{ ...node, stages }] : []
	})
}

export function findProject(tree: CatalogTree, project: string): ProjectNode | undefined {
	return tree.find((node) => node.project.slug === project)
}

function pickApp(stage: StageNode | undefined, preferred: string): App | undefined {
	return stage?.apps.find((app) => app.slug === preferred) ?? stage?.apps[0]
}

/**
 * A dónde lleva el selector de proyecto del breadcrumb: el mismo stage y la misma app si
 * existen allá; si no, la primera app que haya. `null` si el proyecto no tiene apps.
 */
export function switchProjectTarget(
	tree: CatalogTree,
	current: AppRef,
	project: ProjectSlug,
): AppRef | null {
	const node = findProject(tree, project)
	if (!node) return null
	const sameStage = node.stages.find((stage) => stage.stage.slug === current.stage)
	const app =
		pickApp(sameStage, current.app) ??
		node.stages.map((stage) => pickApp(stage, current.app)).find(Boolean)
	return app ? appRefOf(app) : null
}

/** El selector de stage: la misma app en el otro stage o, si no está, la primera. */
export function switchStageTarget(
	tree: CatalogTree,
	current: AppRef,
	stage: StageSlug,
): AppRef | null {
	const stageNode = findProject(tree, current.project)?.stages.find(
		(node) => node.stage.slug === stage,
	)
	const app = pickApp(stageNode, current.app)
	return app ? appRefOf(app) : null
}
