import { resolve } from '$app/paths'
import { redirect } from '@sveltejs/kit'
import type { PageLoad } from './$types'

// Ajustes no tiene una vista propia: abre en la primera sección, "Proxy y DNS".
export const load: PageLoad = () => {
	redirect(307, resolve('/(app)/settings/proxy'))
}
