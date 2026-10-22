import { describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { formatDate } from '@/shared/ui'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import type { AppDomain, OriginCaState } from '../../../../domain'
import { makeDomain } from '../../../../domain/repositories/__fakes__/domainFixtures'
import DomainCard from '../DomainCard.svelte'

// La tarjeta de un dominio con los certificados de Cloudflare (Origin CA, contrato §3.7).
const DAY_MS = 24 * 60 * 60 * 1000
const EXPIRES = new Date(Date.now() + 80 * DAY_MS)

function originCa(overrides: Partial<OriginCaState> = {}): OriginCaState {
	return {
		status: 'active',
		errorCode: null,
		error: null,
		retryAt: null,
		expiresAt: EXPIRES,
		...overrides,
	}
}

const FAILED =
	"muelle no pudo emitir el certificado de Cloudflare: el token no tiene el permiso Zone → SSL and Certificates → Edit en la zona de api.tudominio.cl (Cloudflare: Authentication error); agrégalo al token y pulsa Reintentar; mientras tanto usa Let's Encrypt"

async function renderCard(overrides: Partial<AppDomain>) {
	const onRetry = vi.fn()
	const screen = await render(DomainCard, {
		domain: makeDomain('api.tudominio.cl', overrides),
		busy: false,
		onToggleProxied: vi.fn(),
		onEdit: vi.fn(),
		onAdopt: vi.fn(),
		onRetry,
		onRemove: vi.fn(),
	})
	return { screen, onRetry, text: () => screen.container.textContent ?? '' }
}

const HINTS = [
	m.ingress_origin_ca_issuing(),
	m.ingress_origin_ca_dns_only(),
	m.ingress_origin_ca_dns_only_cloudflare(),
]

describe('DomainCard con los certificados de Cloudflare', () => {
	it('uno que se sirve con Origin CA dice el origen y su vencimiento, y que solo Cloudflare confía en él', async () => {
		const { screen } = await renderCard({
			certSource: 'cloudflare_origin_ca',
			originCa: originCa(),
		})

		const certificate = screen.getByText(
			m.ingress_cert_origin_ca({ date: formatDate(EXPIRES) }),
		)
		await expect.element(certificate).toBeVisible()
		await expect.element(certificate).toHaveAttribute('title', m.ingress_origin_ca_trust())
		await expect.element(screen.getByText(m.ingress_origin_ca_trust())).toBeInTheDocument()
	})

	it("emitiendo: una pista en tono neutro, con Let's Encrypt mientras tanto", async () => {
		const { screen, text } = await renderCard({
			originCa: originCa({ status: 'issuing', expiresAt: null }),
		})

		await expect.element(screen.getByText(m.ingress_origin_ca_issuing())).toBeVisible()
		expect(text()).toContain("Let's Encrypt · vence")
	})

	it('una emisión que falló: el pie con el mensaje del backend y Reintentar', async () => {
		const { screen, onRetry } = await renderCard({
			originCa: originCa({
				status: 'failed',
				errorCode: 'permission',
				error: FAILED,
				retryAt: new Date(Date.now() + 6 * 60 * 60 * 1000),
				expiresAt: null,
			}),
		})

		await expect.element(screen.getByText(`M${FAILED.slice(1)}`)).toBeVisible()
		await screen.getByRole('button', { name: m.common_retry() }).click()
		expect(onRetry).toHaveBeenCalledOnce()
	})

	it('una renovación que falló: sigue con Origin CA, con el pie y Reintentar', async () => {
		const error =
			"muelle no pudo renovar el certificado de Cloudflare, que vence el 19 ene 2027: Cloudflare no respondió (HTTP 503); se reintenta solo; si no lo renueva antes del 5 ene 2027, vuelve a Let's Encrypt"
		const { screen } = await renderCard({
			certSource: 'cloudflare_origin_ca',
			originCa: originCa({ errorCode: 'cloudflare', error, retryAt: new Date() }),
		})

		await expect.element(screen.getByText(`M${error.slice(1)}`)).toBeVisible()
		await expect.element(screen.getByRole('button', { name: m.common_retry() })).toBeVisible()
	})

	it('DNS only en muelle, o en Cloudflare con Proxied prendido: cada uno con su pista', async () => {
		const dnsOnly = originCa({ status: 'dns_only', expiresAt: null })

		const off = await renderCard({ proxied: false, originCa: dnsOnly })
		await expect.element(off.screen.getByText(m.ingress_origin_ca_dns_only())).toBeVisible()
		off.screen.unmount()

		const grey = await renderCard({ proxied: true, originCa: dnsOnly })
		await expect
			.element(grey.screen.getByText(m.ingress_origin_ca_dns_only_cloudflare()))
			.toBeVisible()
	})

	it('sin la opción, o si no califica, no dice nada de Origin CA', async () => {
		for (const state of [null, originCa({ status: 'not_eligible', expiresAt: null })]) {
			const { screen, text } = await renderCard({ originCa: state })
			for (const hint of HINTS) expect(text()).not.toContain(hint)
			expect(text()).not.toContain(m.ingress_origin_ca_trust())
			screen.unmount()
		}
	})
})
