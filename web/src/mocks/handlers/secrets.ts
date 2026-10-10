import type { RequestHandler } from 'msw'

// Handlers de variables de stage y de app. Los llena F1-secrets: `api('/…')` para la URL, `requireSession()`
// para exigir sesión y `apiError(…)` para los errores (ver ../api.ts y ../session.ts).
export const secretsHandlers: RequestHandler[] = []
