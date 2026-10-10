import type { RequestHandler } from 'msw'

// Handlers de dominios (Fase 3). Los llena F1-deploy-ingress: `api('/…')` para la URL, `requireSession()`
// para exigir sesión y `apiError(…)` para los errores (ver ../api.ts y ../session.ts).
export const ingressHandlers: RequestHandler[] = []
