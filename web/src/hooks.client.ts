import { goto } from '$app/navigation'
import { resolve } from '$app/paths'
import type { ClientInit, HandleClientError } from '@sveltejs/kit'
import { authKeys } from '@/modules/auth'
import { configureHttp } from '@/shared/data/http/client'
import { queryClient } from '@/shared/data/query'
import { getLocale } from '@/shared/ui/i18n'
import { describeError } from '@/shared/ui/utils/errorMessage'

// Las pantallas de acceso manejan sus propios 401: desde ahí no se redirige. /2fa no
// está: ahí un 401 es una sesión que se cayó, y va al login como en el resto.
const UNAUTHORIZED_EXEMPT = ['/login', '/setup', '/invite/']
// A /2fa no se redirige desde /2fa.
const TOTP_SETUP_EXEMPT = [...UNAUTHORIZED_EXEMPT, '/2fa']

function onRoute(prefixes: readonly string[]): boolean {
	const path = window.location.pathname
	return prefixes.some((prefix) => path === prefix || path.startsWith(prefix))
}

// El equivalente al `main.tsx` de una SPA: corre UNA vez al arrancar, antes del primer
// `load` y del primer render. Acá se le dice al cliente HTTP qué hacer cuando la sesión
// se cae y se prende MSW si corresponde.
export const init: ClientInit = async () => {
	// El idioma de la página para lectores de pantalla y el corrector (cambiar de idioma
	// recarga la página y vuelve a pasar por acá).
	document.documentElement.lang = getLocale()

	configureHttp({
		onUnauthorized: () => {
			// Lo que hay en el cache era de la sesión que se cayó.
			queryClient.clear()
			if (!onRoute(UNAUTHORIZED_EXEMPT)) void goto(resolve('/login'))
		},
		onTotpSetupRequired: () => {
			// El `me` del cache dice que no hacía falta: ya no es cierto.
			void queryClient.invalidateQueries({ queryKey: authKeys.me() })
			if (!onRoute(TOTP_SETUP_EXEMPT)) void goto(resolve('/2fa'))
		},
	})

	if (import.meta.env.VITE_USE_MOCKS === '1') {
		const { startMocks } = await import('@/mocks/browser')
		await startMocks()
	}
}

// Un error que se escapa de un `load` (backend caído, dos cookies de sesión) llega a
// +error.svelte con un texto que dice qué hacer, no con "Internal Error".
export const handleError: HandleClientError = ({ error }) => ({ message: describeError(error) })
