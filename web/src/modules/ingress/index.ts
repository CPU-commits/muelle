// Superficie pública del módulo. Otros módulos y las rutas importan SOLO de acá.
//
// El estado del proxy para otros módulos: Deploys (la advertencia del paso 7) usa
// `proxyQuery` y `proxyNotice` (null si anda bien, o si `GET /proxy` falló); Ajustes,
// `proxyFollowQuery`, que después de cambiar los puertos sigue preguntando aunque
// `GET /proxy` falle, y `portsRollback` (muelle volvió a los puertos anteriores).
export { ingressUseCases } from './container'
export { domainsQuery, ingressKeys, proxyFollowQuery, proxyQuery } from './data'
export type {
	AppDomain,
	DomainState,
	Hostname,
	PortsRollback,
	ProxyNotice,
	ProxyPorts,
	ProxyStatus,
} from './domain'
export { domainState, proxyNotice } from './domain'
export type { DomainsProps } from './ui'
export { DomainsScreen } from './ui'
