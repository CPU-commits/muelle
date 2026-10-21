// Superficie pública del módulo. Otros módulos y las rutas importan SOLO de acá.
//
// Para Proyectos: `AppDeployState` (estado, imagen y último deploy de una fila, con "—" si
// no se pudo saber). La ruta `(app)/+page.svelte` se lo pasa a `ProjectsScreen` de catalog.
export { deployUseCases } from './container'
export { appStatusQuery, deployKeys, deploysQuery, lastDeployQuery } from './data'
export type {
	AppHealth,
	AppKind,
	AppRef,
	AppStatus,
	Deploy,
	DeployId,
	DeployStatus,
	ServiceStatus,
} from './domain'
export { appHealth, currentDeploy, latestDeploy, replicaCount } from './domain'
export type { DeploysProps, LogsProps, SummaryProps } from './ui'
export { AppDeployState, DeploysScreen, LogsScreen, SummaryScreen } from './ui'
