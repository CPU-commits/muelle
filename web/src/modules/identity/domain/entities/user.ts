/** Una cuenta del panel (`GET /users`). Todas tienen los mismos permisos. */
export interface User {
	readonly username: string
	readonly totpEnabled: boolean
	readonly createdAt: Date
}

/** Si la fila es la del usuario de la sesión: a sí mismo no se borra ni se le quita 2FA. */
export function isCurrentUser(user: User, currentUsername: string | null): boolean {
	return currentUsername !== null && user.username === currentUsername
}
