// Los puertos donde `pnpm test:e2e` sirve sus dos builds (playwright.config.ts). Se pueden
// cambiar para correr varios a la vez, uno por worktree, sin chocar: con el mismo puerto,
// Playwright reusa el servidor que ya está escuchando, aunque sea el build de otro.
//
//   E2E_MOCK_PORT=4183 E2E_PROD_PORT=4184 pnpm test:e2e

function port(name: string, fallback: number): number {
	const raw = process.env[name]
	if (raw === undefined || raw === '') return fallback
	const value = Number(raw)
	if (!Number.isInteger(value) || value < 1 || value > 65535) {
		throw new Error(`${name} tiene que ser un puerto (1-65535), no "${raw}"`)
	}
	return value
}

/** El build con MSW: el `baseURL` de todos los tests. */
export const MOCK_PORT = port('E2E_MOCK_PORT', 4173)
/** El build sin MSW, igual al de `pnpm build`. */
export const PROD_PORT = port('E2E_PROD_PORT', 4174)

if (MOCK_PORT === PROD_PORT) {
	throw new Error(`E2E_MOCK_PORT y E2E_PROD_PORT no pueden ser el mismo (${MOCK_PORT})`)
}

export const MOCK_URL = `http://127.0.0.1:${MOCK_PORT}`
export const PROD_URL = `http://127.0.0.1:${PROD_PORT}`
