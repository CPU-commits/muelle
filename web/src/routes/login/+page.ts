import { resolve } from '$app/paths'
import { redirect } from '@sveltejs/kit'
import { queryClient } from '@/shared/data/query'
import { landingFor, loadSessionState, type LoginStep, type SessionState } from '@/modules/auth'
import type { PageLoad } from './$types'

// Con sesión, /login no tiene nada que hacer. Con una sesión que espera el código (se
// recargó la página en el segundo paso), arranca en el código.
export const load: PageLoad = async () => {
	let state: SessionState
	try {
		state = await loadSessionState(queryClient)
	} catch {
		// Backend caído: igual se muestra el formulario, y el login dirá qué pasa.
		return { step: 'password' as LoginStep }
	}
	if (state.status === 'signed_in') {
		redirect(307, landingFor(state.me) === 'totp_setup' ? resolve('/2fa') : resolve('/'))
	}
	return { step: (state.status === 'mfa_pending' ? 'code' : 'password') as LoginStep }
}
