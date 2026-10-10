import { goto } from '$app/navigation'
import { resolve } from '$app/paths'
import type { ClientInit } from '@sveltejs/kit'
import { configureHttp } from '@/shared/data/http/client'
import { queryClient } from '@/shared/data/query'

// Las pantallas de acceso manejan sus propios 401: desde ahí no se redirige.
const ACCESS_PREFIXES = ['/login', '/setup', '/invite/', '/2fa']

function onAccessRoute(): boolean {
	const path = window.location.pathname
	return ACCESS_PREFIXES.some((prefix) => path === prefix || path.startsWith(prefix))
}

// El equivalente al `main.tsx` de una SPA: corre UNA vez al arrancar, antes del primer
// `load` y del primer render. Acá se le dice al cliente HTTP qué hacer cuando la sesión
// se cae y se prende MSW si corresponde.
export const init: ClientInit = async () => {
	configureHttp({
		onUnauthorized: () => {
			// Lo que hay en el cache era de la sesión que se cayó.
			queryClient.clear()
			if (!onAccessRoute()) void goto(resolve('/login'))
		},
		onTotpSetupRequired: () => {
			if (!onAccessRoute()) void goto(resolve('/2fa'))
		},
	})

	if (import.meta.env.VITE_USE_MOCKS === '1') {
		const { startMocks } = await import('@/mocks/browser')
		await startMocks()
	}
}
