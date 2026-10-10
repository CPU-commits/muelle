import type { RequestHandler } from 'msw'

// Handlers de proyectos, stages y apps. Los llena F1-catalog: `api('/…')` para la URL, `requireSession()`
// para exigir sesión y `apiError(…)` para los errores (ver ../api.ts y ../session.ts).
export const catalogHandlers: RequestHandler[] = []
