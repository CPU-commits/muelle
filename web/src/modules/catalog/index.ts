// Superficie pública del módulo. Otros módulos y las rutas importan SOLO de acá.
export { catalogUseCases } from './container'
export { appQuery, appSummaryQuery, catalogKeys, catalogTreeQuery } from './data'
export type {
	App,
	AppKind,
	AppRef,
	AppSlug,
	AppSummary,
	CatalogTree,
	DeployState,
	Project,
	ProjectSlug,
	Stage,
	StageSlug,
} from './domain'
export { appRefFrom, appStackName, stageNetwork } from './domain'
export { AppBreadcrumb, AppChangesBadge, AppConfigScreen, AppIdentity, ProjectsScreen } from './ui'
