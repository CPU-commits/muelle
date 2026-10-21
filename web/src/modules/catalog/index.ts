// Superficie pública del módulo. Otros módulos y las rutas importan SOLO de acá.
export { catalogUseCases } from './container'
export { appQuery, appSummaryQuery, appTargetsQuery, catalogKeys, catalogTreeQuery } from './data'
export type {
	App,
	AppKind,
	AppRef,
	AppService,
	AppSlug,
	AppSummary,
	CatalogTree,
	DeployState,
	Project,
	ProjectSlug,
	Stage,
	StageSlug,
} from './domain'
export { appRefFrom, appStackName, serviceTargets, stageNetwork } from './domain'
export type { AppRowDeploy, AppRowDeployFactory } from './ui'
export {
	AppBreadcrumb,
	AppChangesBadge,
	AppConfigScreen,
	AppIdentity,
	ProjectsScreen,
	StageBreadcrumb,
} from './ui'
