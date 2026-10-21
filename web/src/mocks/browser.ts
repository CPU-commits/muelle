import { setupWorker } from 'msw/browser'
import { handlers } from './handlers'

export const worker = setupWorker(...handlers)

// Lo que no tiene handler (los assets, el HMR de Vite) pasa de largo sin avisar.
export async function startMocks(): Promise<void> {
	await worker.start({
		onUnhandledRequest: 'bypass',
		quiet: true,
	})
}
