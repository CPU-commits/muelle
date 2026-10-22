// El puerto donde `pnpm test:e2e` sirve su build (playwright.config.ts). Adentro del
// network namespace de `scripts/isolated-net.mjs` es de esa corrida. Con la red del host
// (`MUELLE_TEST_HOST_NET=1`, o sin `unshare`) se cambia para correr varios a la vez, uno
// por worktree, sin chocar:
//
//   E2E_MOCK_PORT=4183 pnpm test:e2e

function port(name: string, fallback: number): number {
	const raw = process.env[name]
	if (raw === undefined || raw === '') return fallback
	const value = Number(raw)
	if (!Number.isInteger(value) || value < 1 || value > 65535) {
		throw new Error(`${name} tiene que ser un puerto (1-65535), no "${raw}"`)
	}
	return value
}

/** El build con MSW: el `baseURL` del test. */
export const MOCK_PORT = port('E2E_MOCK_PORT', 4173)

export const MOCK_URL = `http://127.0.0.1:${MOCK_PORT}`
