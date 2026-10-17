// Superficie pública del módulo. Otros módulos y las rutas importan SOLO de acá.
//
// El estado del proxy para otros módulos: Deploys (la advertencia del paso 7) usa
// `proxyQuery` y `proxyNotice` (null si anda bien, o sin la Fase 3); Ajustes, después de
// cambiar los puertos, `proxyFollowQuery`, que sigue preguntando aunque `GET /proxy` falle.
export { ingressUseCases } from './container'
export { domainsQuery, ingressKeys, proxyFollowQuery, proxyQuery } from './data'
export type { AppDomain, DomainState, Hostname, ProxyNotice, ProxyStatus } from './domain'
export { domainState, proxyNotice } from './domain'
export type { DomainsProps } from './ui'
export { DomainsScreen } from './ui'
