import { httpCatalogRepository } from './data/repositories/httpCatalogRepository'
import {
	makeCreateApp,
	makeCreateProject,
	makeCreateStage,
	makeDeleteApp,
	makeDeleteProject,
	makeDeleteStage,
	makeGetApp,
	makeListCatalog,
	makeRenameProject,
	makeUpdateApp,
} from './domain'

// Composition root del módulo: el ÚNICO lugar que conoce el repositorio concreto.
const deps = { catalog: httpCatalogRepository }

export const catalogUseCases = {
	listCatalog: makeListCatalog(deps),
	createProject: makeCreateProject(deps),
	renameProject: makeRenameProject(deps),
	deleteProject: makeDeleteProject(deps),
	createStage: makeCreateStage(deps),
	deleteStage: makeDeleteStage(deps),
	getApp: makeGetApp(deps),
	createApp: makeCreateApp(deps),
	updateApp: makeUpdateApp(deps),
	deleteApp: makeDeleteApp(deps),
}
