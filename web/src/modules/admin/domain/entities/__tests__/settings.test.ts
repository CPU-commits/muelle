import { describe, expect, it } from 'vitest'
import { movedPanelOrigin, panelOrigin, type ProxySettings, updatePanelOriginCommand } from '../..'

const PROXY: ProxySettings = {
	cloudflare: { tokenSet: false, zones: null },
	publicIpv4: '203.0.113.24',
	publicIpv6: '',
	acmeEmail: 'ops@tudominio.cl',
	panelDomain: 'panel.tudominio.cl',
	httpPort: 80,
	httpsPort: 443,
}

// Dónde sirve muelle-caddy el panel: `https://<panel_domain>[:<proxy_https_port>]`, sin el
// puerto si es 443 (contrato §0.1 y §3.5).
describe('settings', () => {
	it('el origen del panel lleva el puerto HTTPS solo si no es 443', () => {
		expect(panelOrigin(PROXY)).toBe('https://panel.tudominio.cl')
		expect(panelOrigin({ ...PROXY, httpsPort: 8443 })).toBe('https://panel.tudominio.cl:8443')
	})

	it('sin dominio del panel (una IP o localhost, en desarrollo) no hay origen de Caddy', () => {
		expect(panelOrigin({ ...PROXY, panelDomain: '' })).toBeNull()
	})

	it('el panel se muda solo si cambió el puerto HTTPS y hay dominio del panel', () => {
		const https = { ...PROXY, httpsPort: 8443 }
		expect(movedPanelOrigin(PROXY, https)).toBe('https://panel.tudominio.cl:8443')
		expect(movedPanelOrigin(https, PROXY)).toBe('https://panel.tudominio.cl')
		// Solo el HTTP: el panel sigue donde está.
		expect(movedPanelOrigin(PROXY, { ...PROXY, httpPort: 8080 })).toBeNull()
		expect(movedPanelOrigin(PROXY, PROXY)).toBeNull()
		expect(
			movedPanelOrigin({ ...PROXY, panelDomain: '' }, { ...https, panelDomain: '' }),
		).toBeNull()
	})

	it('el comando que actualiza MUELLE_PANEL_ORIGIN en el servidor (contrato §5.2)', () => {
		expect(updatePanelOriginCommand('https://panel.tudominio.cl:8443')).toBe(
			'docker service update --env-add MUELLE_PANEL_ORIGIN=https://panel.tudominio.cl:8443 muelle_muelle',
		)
	})
})
