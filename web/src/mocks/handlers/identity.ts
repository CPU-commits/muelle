import type { RequestHandler } from 'msw'

// Handlers de usuarios, invitaciones y tokens. Los llena F1-identity-admin: `api('/…')` para la URL, `requireSession()`
// para exigir sesión y `apiError(…)` para los errores (ver ../api.ts y ../session.ts).
export const identityHandlers: RequestHandler[] = []
