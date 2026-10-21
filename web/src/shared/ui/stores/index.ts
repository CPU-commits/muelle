// La sesión de muelle es una cookie HttpOnly: el panel no la lee ni la guarda. Quién es
// el usuario lo dice `GET /auth/me` (el barrel de `auth` exporta `meQuery`), así que acá
// solo queda el estado puramente de la vista.
export type { ToastItem, ToastTone } from './toasts.svelte.ts'
export { toast, toasts } from './toasts.svelte.ts'
