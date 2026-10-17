// La vuelta a los puertos anteriores que se cerró en este navegador. `GET /proxy` trae
// `ports_rollback` hasta el próximo cambio de puertos o un reinicio de muelle (contrato §3.3),
// quizás días: cerrado, el aviso de esa vuelta (su `at`) no vuelve a salir; el de una nueva,
// sí. Va en el localStorage de este origen (ver `noticeStorage.ts`).

import { readNotice, writeNotice } from './noticeStorage'

const STORAGE_KEY = 'muelle:admin:ports-rollback-dismissed'

/** El `at` (en ms) de la vuelta que se cerró; null si no hay, o si lo guardado no es un número. */
export function loadDismissedRollback(): number | null {
	const saved = readNotice(STORAGE_KEY)
	if (saved === null || !/^\d+$/.test(saved)) return null
	const at = Number(saved)
	return Number.isSafeInteger(at) ? at : null
}

export function saveDismissedRollback(at: Date): void {
	writeNotice(STORAGE_KEY, String(at.getTime()))
}
