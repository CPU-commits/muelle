import { describe, expect, it } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import type { App } from '../../../../domain'
import {
	fakeComposeApp,
	fakeServiceApp,
} from '../../../../domain/repositories/__fakes__/inMemoryCatalogRepository'
import type { AppRowDeploy, AppRowDeployFactory } from '../appRowDeploy'
import AppRowHarness from './AppRowHarness.svelte'

const api = fakeServiceApp('auditfy', 'staging', 'api', {
	spec: { ...fakeServiceApp('a', 'b', 'c').spec, image: 'ghcr.io/aumi-app/backend-app:staging' },
	lastDeployedAt: new Date(Date.now() - 12 * 60_000),
})

const RUNNING: AppRowDeploy = {
	tone: 'ok',
	statusText: 'Corriendo · 2/2',
	image: 'ghcr.io/aumi-app/backend-app:sha-3f9c2e1',
	lastDeployText: '#143 · hace 12 min',
	lastDeployFailed: false,
	lastDeployFailedText: 'falló',
}

interface Call {
	readonly app: App
	readonly enabled: () => boolean
}

/** Un `appDeploy` que devuelve `view` y anota con qué app y qué `enabled` lo llamaron. */
function fakeDeploy(view: Partial<AppRowDeploy> = {}) {
	const calls: Call[] = []
	const factory: AppRowDeployFactory = (app, enabled) => {
		calls.push({ app: app(), enabled })
		return { ...RUNNING, ...view }
	}
	return { factory, calls }
}

const rowText = (slug: string) =>
	(document.querySelector(`a[href="#${slug}"]`)?.textContent ?? '').replace(/\s+/g, ' ')

describe('AppRow', () => {
	it('sin el estado de deploy (la ruta no lo pasa) muestra lo del catálogo: el tipo, la imagen del spec y cuándo se desplegó', async () => {
		const never = fakeServiceApp('auditfy', 'staging', 'nueva')
		const screen = await render(AppRowHarness, { apps: [api, never] })

		expect(rowText('api')).toContain(m.catalog_app_kind_service())
		expect(rowText('api')).toContain('ghcr.io/aumi-app/backend-app:staging')
		expect(rowText('nueva')).toContain(m.catalog_app_no_deploys())
		await expect
			.element(screen.getByRole('img', { name: m.catalog_badge_never() }))
			.toBeInTheDocument()
	})

	it('con la Fase 2 muestra el estado, la imagen que corre y el último deploy', async () => {
		const { factory, calls } = fakeDeploy()
		await render(AppRowHarness, { apps: [api], deploy: factory })

		const text = rowText('api')
		expect(text).toContain('Corriendo · 2/2')
		expect(text).toContain('ghcr.io/aumi-app/backend-app:sha-3f9c2e1')
		expect(text).not.toContain('ghcr.io/aumi-app/backend-app:staging')
		expect(text).toContain('#143 · hace 12 min')
		expect(text).not.toContain(m.catalog_app_kind_service())
		// Una sola vez por fila, con la app de la fila.
		expect(calls).toHaveLength(1)
		expect(calls[0]?.app.slug).toBe('api')
	})

	it('un último deploy fallido suma "· falló" en rojo', async () => {
		const { factory } = fakeDeploy({
			lastDeployText: '#58 · hace 40 min',
			lastDeployFailed: true,
		})
		const screen = await render(AppRowHarness, { apps: [api], deploy: factory })

		expect(rowText('api')).toContain('#58 · hace 40 min · falló')
		const failed = screen.getByText('falló').element()
		const plain = screen.getByText('#58 · hace 40 min', { exact: false }).element()
		expect(getComputedStyle(failed).color).not.toBe(getComputedStyle(plain).color)
	})

	it('el texto del estado va del color de su punto: una caída en rojo, uno al día en gris', async () => {
		const down = fakeServiceApp('auditfy', 'staging', 'down')
		const views: Record<string, Partial<AppRowDeploy>> = {
			api: {},
			down: { tone: 'bad', statusText: 'Caída · 0/1' },
		}
		const factory: AppRowDeployFactory = (app) => ({ ...RUNNING, ...views[app().slug] })
		const screen = await render(AppRowHarness, { apps: [api, down], deploy: factory })

		const bad = getComputedStyle(screen.getByText('Caída · 0/1').element()).color
		const ok = getComputedStyle(screen.getByText('Corriendo · 2/2').element()).color
		expect(bad).not.toBe(ok)
	})

	it('si la Fase 2 no sabe la imagen, queda la del spec (o los servicios del compose)', async () => {
		const compose = fakeComposeApp(
			'aumi-agents',
			'staging',
			'agents',
			'services:\n  gateway:\n    image: x/gateway\n  runner:\n    image: x/runner\n',
		)
		const { factory } = fakeDeploy({ image: null, statusText: '—', lastDeployText: '—' })
		await render(AppRowHarness, { apps: [api, compose], deploy: factory })

		expect(rowText('api')).toContain('ghcr.io/aumi-app/backend-app:staging')
		expect(rowText('agents')).toContain('gateway · runner')
	})

	it('pide el estado recién cuando la fila se ve, y una sola vez', async () => {
		const { factory, calls } = fakeDeploy()
		await render(AppRowHarness, { apps: [api], deploy: factory, spacer: 4000 })

		expect(calls).toHaveLength(1)
		const enabled = calls[0]?.enabled ?? (() => true)
		// Abajo de la pantalla: todavía no.
		await new Promise((resolve) => setTimeout(resolve, 100))
		expect(enabled()).toBe(false)

		document.querySelector('a[href="#api"]')?.scrollIntoView()
		await expect.poll(enabled).toBe(true)
		// Volver arriba no lo apaga: el dato ya está pedido.
		window.scrollTo(0, 0)
		await new Promise((resolve) => setTimeout(resolve, 100))
		expect(enabled()).toBe(true)
		expect(calls).toHaveLength(1)
	})
})
