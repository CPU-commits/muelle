import { beforeEach, describe, expect, it, vi } from 'vitest'
import { userEvent } from 'vitest/browser'
import { render } from 'vitest-browser-svelte'
import { appOwner, makeGetEnv, makePatchEnv } from '../../../../domain'
import { InMemoryEnvRepository } from '../../../../domain/repositories/__fakes__/inMemoryEnvRepository'
import ImportPanelHarness from './ImportPanelHarness.svelte'

const mocks = vi.hoisted(() => ({
	getEnv: vi.fn(),
	patchEnv: vi.fn(),
}))

// El container con los casos de uso de verdad sobre el fake en memoria del puerto.
vi.mock('@/modules/secrets/container', () => ({
	secretsUseCases: { getEnv: mocks.getEnv, replaceEnv: vi.fn(), patchEnv: mocks.patchEnv },
}))

// Lo pegado en "Importar .env" puede ser un secreto: cerrado el panel, ni el textarea ni su
// texto pueden quedar vivos. vitest lanza Chromium con --expose-gc (vite.config.ts), y el
// patrón es el de "Modal no retiene lo que mostró" (shared/ui/…/modal/__tests__).
describe('EnvImportPanel no retiene lo pegado', () => {
	const gc = (globalThis as { gc?: () => void }).gc

	/**
	 * Fuerza el GC hasta que `ref` quede vacía (o se acabe el tiempo). Cada intento es una
	 * tarea nueva y el GC va antes del `deref`: una WeakRef leída mantiene vivo su objeto
	 * hasta el final de la tarea. Con toda la suite corriendo, algún cuadro o timer de la
	 * página puede tardar en soltar su referencia: por eso se reintenta, no se espera fijo.
	 */
	async function expectCollected(ref: WeakRef<object>): Promise<void> {
		await vi.waitFor(
			() => {
				gc?.()
				expect(ref.deref()).toBeUndefined()
			},
			{ timeout: 5000, interval: 50 },
		)
	}

	beforeEach(() => {
		vi.clearAllMocks()
		const fake = new InMemoryEnvRepository({
			owners: { 'app:auditfy/staging/api': [{ key: 'PORT', value: '3000', secret: false }] },
		})
		mocks.getEnv.mockImplementation(makeGetEnv({ env: fake }))
		mocks.patchEnv.mockImplementation(makePatchEnv({ env: fake }))
	})

	it('después de confirmar un import, el textarea y su texto se liberan', async () => {
		expect(gc, 'falta --expose-gc en el Chromium de vitest').toBeTypeOf('function')
		const screen = await render(ImportPanelHarness, {
			owner: appOwner('auditfy', 'staging', 'api'),
		})

		await screen.getByRole('button', { name: 'Importar .env' }).click()
		const textarea = screen.getByRole('textbox', { name: 'Contenido del .env' })
		// "Marcar como secretas" está prendido: volver secreta a PORT pide confirmar.
		await textarea.fill('PORT=marcador-secreto-pegado')
		const ref = new WeakRef(textarea.element())
		await screen.getByRole('button', { name: 'Importar', exact: true }).click()
		await screen.getByRole('dialog').getByRole('button', { name: 'Importar' }).click()
		await expect.element(screen.getByRole('dialog')).not.toBeInTheDocument()
		await expect
			.element(screen.getByRole('textbox', { name: 'Contenido del .env' }))
			.not.toBeInTheDocument()

		// Svelte guarda el último evento delegado (con su target) hasta el próximo: un clic
		// cualquiera lo suelta, como pasa al seguir usando la página.
		await userEvent.click(screen.getByRole('heading', { name: 'Variables' }))

		await expectCollected(ref)
	})
})
