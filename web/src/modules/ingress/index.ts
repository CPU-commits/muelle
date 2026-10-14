// Superficie pública del módulo. Otros módulos y las rutas importan SOLO de acá.
//
// Para Ajustes (F1-identity-admin), si quiere el aviso "proxy desincronizado": `proxyQuery`
// y `proxyNotice` (null si anda bien, o sin la Fase 3).
export { ingressUseCases } from './container'
export { domainsQuery, ingressKeys, proxyQuery } from './data'
export type { AppDomain, DomainState, Hostname, ProxyNotice, ProxyStatus } from './domain'
export { domainState, proxyNotice } from './domain'
export type { DomainsProps } from './ui'
export { DomainsScreen } from './ui'
