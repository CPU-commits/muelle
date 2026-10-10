import { InvalidPort } from '../errors/adminErrors'

/** Dónde escucha muelle-caddy en el host (`proxy_http_port`, `proxy_https_port`). */
export interface ProxyPorts {
	readonly http: number
	readonly https: number
}

const PORT = /^\d{1,5}$/

function parsePort(raw: string, field: 'httpPort' | 'httpsPort'): number {
	const text = raw.trim()
	const port = PORT.test(text) ? Number(text) : Number.NaN
	if (!Number.isInteger(port) || port < 1 || port > 65535) throw new InvalidPort(field, 'format')
	return port
}

export function parseProxyPorts(http: string, https: string): ProxyPorts {
	const ports = { http: parsePort(http, 'httpPort'), https: parsePort(https, 'httpsPort') }
	if (ports.http === ports.https) throw new InvalidPort('httpsPort', 'same')
	return ports
}
