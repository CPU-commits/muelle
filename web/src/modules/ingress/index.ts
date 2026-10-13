// Superficie pública del módulo. Otros módulos y las rutas importan SOLO de acá.
export { ingressUseCases } from './container'
export { domainsQuery, ingressKeys } from './data'
export type { AppDomain, DomainState, Hostname } from './domain'
export { domainState } from './domain'
export type { DomainsProps } from './ui'
export { DomainsScreen } from './ui'
