import type { App, AppRef } from '../entities/app'
import type { Project } from '../entities/project'
import type { Stage } from '../entities/stage'
import type { AppSpec } from '../value-objects/appSpec'
import type { AppSlug, ProjectSlug, StageSlug } from '../value-objects/slugs'

export interface NewProject {
	readonly slug: ProjectSlug
	readonly name: string
}

/** Una app nueva: de formulario con su spec, o compose con su YAML. */
export type NewApp = {
	readonly project: ProjectSlug
	readonly stage: StageSlug
	readonly slug: AppSlug
	/** Sin él, el backend usa 300 s. */
	readonly deployTimeoutSeconds?: number
} & (
	| { readonly kind: 'service'; readonly spec: AppSpec }
	| { readonly kind: 'compose'; readonly composeYaml: string }
)

/** Lo que cambia un PATCH de la app. Lo que falta no se toca. */
export interface AppChanges {
	readonly spec?: AppSpec
	readonly composeYaml?: string
	readonly deployTimeoutSeconds?: number
}

// El puerto del catálogo. La implementación HTTP traduce los `error.code` del backend a
// los errores de `errors/catalogErrors.ts`; ningún método devuelve un status.
export interface CatalogRepository {
	listProjects(signal?: AbortSignal): Promise<Project[]>
	createProject(input: NewProject): Promise<Project>
	renameProject(project: ProjectSlug, name: string): Promise<Project>
	/** `ProjectNotEmpty` si todavía tiene stages. */
	deleteProject(project: ProjectSlug): Promise<void>

	listStages(project: ProjectSlug, signal?: AbortSignal): Promise<Stage[]>
	createStage(project: ProjectSlug, slug: StageSlug): Promise<Stage>
	/** `StageNotEmpty` si todavía tiene apps. */
	deleteStage(project: ProjectSlug, stage: StageSlug): Promise<void>

	listApps(project: ProjectSlug, stage: StageSlug, signal?: AbortSignal): Promise<App[]>
	getApp(ref: AppRef, signal?: AbortSignal): Promise<App>
	createApp(input: NewApp): Promise<App>
	/** `KindMismatch` si se manda el spec de una compose o el compose de una de formulario. */
	updateApp(ref: AppRef, changes: AppChanges): Promise<App>
	deleteApp(ref: AppRef): Promise<void>
}

export interface CatalogDeps {
	readonly catalog: CatalogRepository
}
