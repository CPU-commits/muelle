import type { PageLoad } from './$types'

// `muelle serve` imprime `<panel>/setup?token=…` al arrancar sin usuarios.
export const load: PageLoad = ({ url }) => ({ token: url.searchParams.get('token') })
