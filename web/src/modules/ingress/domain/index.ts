export type {
	AppDomain,
	Certificate,
	DnsMode,
	DnsRecord,
	DomainChanges,
	DomainState,
	NewDomain,
	OriginCaNotice,
} from './entities/appDomain'
export {
	canToggleProxied,
	CERT_WARNING_DAYS,
	certificateExpiresSoon,
	dnsMode,
	domainState,
	failingDomains,
	hasProvisioning,
	hasWaitingDns,
	isReachable,
	manualRecords,
	originCaNotice,
	publicAddress,
	replaceDomain,
	sortDomains,
	suggestedTargets,
} from './entities/appDomain'
export type { OriginCaState } from './entities/originCa'
export {
	DomainNotFound,
	CloudflareUnavailable,
	DockerUnavailable,
	HostnameTaken,
	NothingToAdopt,
} from './errors/ingressErrors'
export type {
	CaddyState,
	PanelCertificate,
	PortsRollback,
	ProxyNotice,
	ProxyPorts,
	ProxyStatus,
	ProxySyncStatus,
} from './entities/proxyStatus'
export { PROXY_SYNC_STATUSES, proxyNotice, proxySettling } from './entities/proxyStatus'
export type { DomainRepository, IngressDeps } from './repositories/domainRepository'
export type { ProxyDeps, ProxyRepository } from './repositories/proxyRepository'
export type { GetProxyStatus } from './use-cases/getProxyStatus'
export { makeGetProxyStatus } from './use-cases/getProxyStatus'
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
	CertSource,
	DnsRecordType,
	DomainErrorKind,
	DomainStatus,
	OriginCaErrorCode,
	OriginCaStatus,
	TlsMode,
} from './value-objects/domainStatus'
export {
	CERT_SOURCES,
	DNS_RECORD_TYPES,
	DOMAIN_ERROR_KINDS,
	DOMAIN_STATUSES,
	ORIGIN_CA_ERROR_CODES,
	ORIGIN_CA_STATUSES,
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
