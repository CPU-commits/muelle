import { httpDeployRepository } from './data/repositories/httpDeployRepository'
import {
	makeCreateDeploy,
	makeFollowDeployLog,
	makeGetAppStatus,
	makeGetDeploy,
	makeListDeploys,
	makeRollbackDeploy,
	makeStreamAppLogs,
	realWait,
} from './domain'

// Composition root del módulo: el ÚNICO lugar que conoce el repositorio concreto.
const deps = { deploys: httpDeployRepository, wait: realWait }

export const deployUseCases = {
	getAppStatus: makeGetAppStatus(deps),
	listDeploys: makeListDeploys(deps),
	getDeploy: makeGetDeploy(deps),
	createDeploy: makeCreateDeploy(deps),
	rollback: makeRollbackDeploy(deps),
	followDeployLog: makeFollowDeployLog(deps),
	streamAppLogs: makeStreamAppLogs(deps),
}
