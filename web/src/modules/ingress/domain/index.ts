export type {
	AppDomain,
	Certificate,
	DnsMode,
	DnsRecord,
	DomainChanges,
	DomainState,
	NewDomain,
} from './entities/appDomain'
export {
	canToggleProxied,
	CERT_WARNING_DAYS,
	certificateExpiresSoon,
	dnsMode,
	domainState,
	failingDomains,
	hasProvisioning,
	isReachable,
	manualRecords,
	publicAddress,
	replaceDomain,
	sortDomains,
	suggestedTargets,
} from './entities/appDomain'
export { DomainNotFound, HostnameTaken, NothingToAdopt } from './errors/ingressErrors'
export type { DomainRepository, IngressDeps } from './repositories/domainRepository'
export type { AddDomain, AddDomainInput } from './use-cases/addDomain'
export { makeAddDomain } from './use-cases/addDomain'
export type { AdoptRecord } from './use-cases/adoptRecord'
export { makeAdoptRecord } from './use-cases/adoptRecord'
export type { ChangeTarget, ChangeTargetInput } from './use-cases/changeTarget'
export { makeChangeTarget } from './use-cases/changeTarget'
export type { ListDomains } from './use-cases/listDomains'
export { makeListDomains } from './use-cases/listDomains'
export type { DomainRef, RemoveDomain } from './use-cases/removeDomain'
export { makeRemoveDomain } from './use-cases/removeDomain'
export type { RetryDomain } from './use-cases/retryDomain'
export { makeRetryDomain } from './use-cases/retryDomain'
export type { SetProxied, SetProxiedInput } from './use-cases/setProxied'
export { makeSetProxied } from './use-cases/setProxied'
export type { AppRef } from './value-objects/appRef'
export type { DomainTarget, TargetProblem } from './value-objects/domainTarget'
export { formatTarget, parseTarget, sameTarget, targetProblem } from './value-objects/domainTarget'
export type {
	DnsRecordType,
	DomainErrorKind,
	DomainStatus,
	TlsMode,
} from './value-objects/domainStatus'
export {
	DNS_RECORD_TYPES,
	DOMAIN_ERROR_KINDS,
	DOMAIN_STATUSES,
	TLS_MODES,
} from './value-objects/domainStatus'
export type { Hostname, HostnameProblem } from './value-objects/hostname'
export {
	asHostname,
	HOSTNAME_MAX_LENGTH,
	hostnameProblem,
	normalizeHostname,
	parseHostname,
} from './value-objects/hostname'
