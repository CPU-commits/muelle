import { afterEach, describe, expect, it, vi } from 'vitest'
import { http } from '@/shared/data/http/client'
import { adminHttpDataSource, SETTINGS_PATCH_TIMEOUT_MS } from '../adminHttpDataSource'

// Un `GET /settings` completo (contrato §4: todas las claves, siempre).
const SETTINGS = {
	require_2fa: false,
	cloudflare_token_set: false,
	cloudflare_zones: null,
	cloudflare_origin_ca: false,
	public_ip: '',
	public_ipv6: '',
	acme_email: '',
	panel_domain: '',
	proxy_http_port: 80,
	proxy_https_port: 443,
	panel_allowed_cidrs: [],
	client_ip: '127.0.0.1',
}

describe('adminHttpDataSource', () => {
	afterEach(() => vi.restoreAllMocks())

	it('el PATCH de settings espera hasta 150 s: el backend aplica el proxy antes de responder', async () => {
		const patch = vi.spyOn(http, 'patch').mockResolvedValue(SETTINGS)

		await adminHttpDataSource.patchSettings({ proxy_http_port: 8080 })

		expect(SETTINGS_PATCH_TIMEOUT_MS).toBe(150_000)
		expect(patch).toHaveBeenCalledWith('/settings', {
			body: { proxy_http_port: 8080 },
			timeoutMs: SETTINGS_PATCH_TIMEOUT_MS,
		})
	})

	it('el resto sigue con el plazo de siempre', async () => {
		const get = vi.spyOn(http, 'get').mockResolvedValue(SETTINGS)

		await adminHttpDataSource.getSettings()

		expect(get).toHaveBeenCalledWith('/settings', {})
	})
})
