import { httpDomainRepository } from './data/repositories/httpDomainRepository'
import {
	makeAddDomain,
	makeAdoptRecord,
	makeChangeTarget,
	makeListDomains,
	makeRemoveDomain,
	makeRetryDomain,
	makeSetProxied,
} from './domain'

// Composition root del módulo: el ÚNICO lugar que conoce el repositorio concreto.
const deps = { domains: httpDomainRepository }

export const ingressUseCases = {
	listDomains: makeListDomains(deps),
	addDomain: makeAddDomain(deps),
	setProxied: makeSetProxied(deps),
	changeTarget: makeChangeTarget(deps),
	removeDomain: makeRemoveDomain(deps),
	adoptRecord: makeAdoptRecord(deps),
	retryDomain: makeRetryDomain(deps),
}
