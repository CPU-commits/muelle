import { resolve } from '$app/paths'
import { redirect } from '@sveltejs/kit'
import { queryClient } from '@/shared/data/query'
import { loadSessionState } from '@/modules/auth'
import type { PageLoad } from './$types'

// Fuera de `(app)` a propósito: es lo único que puede hacer una sesión con
// `needsTotpSetup`, y el guard de `(app)` la manda acá.
export const load: PageLoad = async () => {
	const state = await loadSessionState(queryClient)
	if (state.status !== 'signed_in') redirect(307, resolve('/login'))
	return { me: state.me }
}
