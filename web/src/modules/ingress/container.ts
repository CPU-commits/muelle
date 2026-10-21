import { httpDomainRepository } from './data/repositories/httpDomainRepository'
import { httpProxyRepository } from './data/repositories/httpProxyRepository'
import {
	makeAddDomain,
	makeAdoptRecord,
	makeChangeTarget,
	makeGetProxyStatus,
	makeListDomains,
	makeRemoveDomain,
	makeRetryDomain,
	makeSetProxied,
} from './domain'

// Composition root del módulo: el ÚNICO lugar que conoce el repositorio concreto.
const deps = { domains: httpDomainRepository, proxy: httpProxyRepository }

export const ingressUseCases = {
	listDomains: makeListDomains(deps),
	addDomain: makeAddDomain(deps),
	setProxied: makeSetProxied(deps),
	changeTarget: makeChangeTarget(deps),
	removeDomain: makeRemoveDomain(deps),
	adoptRecord: makeAdoptRecord(deps),
	retryDomain: makeRetryDomain(deps),
	getProxyStatus: makeGetProxyStatus(deps),
}
