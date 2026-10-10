import type { RequestHandler } from 'msw'

// Handlers de settings y auditoría (más las secciones de Fase 2/3). Los llena F1-identity-admin: `api('/…')` para la URL, `requireSession()`
// para exigir sesión y `apiError(…)` para los errores (ver ../api.ts y ../session.ts).
export const adminHandlers: RequestHandler[] = []
