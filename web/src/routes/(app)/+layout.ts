import { resolve } from '$app/paths'
import { redirect } from '@sveltejs/kit'
import { queryClient } from '@/shared/data/query'
import { landingFor, loadSessionState } from '@/modules/auth'
import type { LayoutLoad } from './$types'

// Guard del grupo con sesión. `(app)` no aparece en la URL: agrupa las rutas que
// comparten este layout y este chequeo. La sesión es una cookie HttpOnly, así que se
// pregunta a `GET /auth/me` (a través del cache de TanStack Query).
export const load: LayoutLoad = async () => {
	const state = await loadSessionState(queryClient)
	if (state.status !== 'signed_in') redirect(307, resolve('/login'))
	if (landingFor(state.me) === 'totp_setup') redirect(307, resolve('/2fa'))
	return { me: state.me }
}
