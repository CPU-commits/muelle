import { loadPersisted, savePersisted } from './persist'

// Los usuarios y la sesión del backend simulado. Es el dueño de los usuarios: el mock
// de `identity` (lista de usuarios, invitaciones) lee y escribe acá.

/** El usuario demo de `pnpm dev:mock`. */
export const MOCK_USERNAME = 'felipe'
export const MOCK_PASSWORD = 'una-contraseña-larga'
/** El único código de 2FA que aceptan los mocks. */
export const MOCK_TOTP_CODE = '123456'
export const MOCK_TOTP_SECRET = 'JBSWY3DPEHPK3PXP'
/** Abre `/setup?token=setup-demo` (con `authDb.reset({ users: [] })` para que funcione). */
export const MOCK_SETUP_TOKEN = 'setup-demo'
/** Abre `/invite/invitacion-demo`. */
export const MOCK_INVITE_TOKEN = 'invitacion-demo'

export const MAX_FAILED_LOGINS = 5
export const MAX_BAD_CODES = 5
export const LOCK_MS = 60_000

export interface MockUser {
	username: string
	password: string
	totpEnabled: boolean
	createdAt: string
}

export interface MockSession {
	username: string
	mfaPending: boolean
	badCodes: number
}

export interface AuthState {
	users: MockUser[]
	session: MockSession | null
	/** Usuario con un secreto TOTP pendiente de confirmar. */
	pendingTotpFor: string | null
	invites: string[]
	setupToken: string
	/** `require_2fa` de settings: sin 2FA, la sesión solo puede configurarla. */
	require2fa: boolean
	failedLogins: number
	lockedUntil: number
}

const STORAGE_KEY = 'auth'

function seed(): AuthState {
	return {
		users: [
			{
				username: MOCK_USERNAME,
				password: MOCK_PASSWORD,
				totpEnabled: false,
				createdAt: '2026-10-09T12:00:00.000Z',
			},
		],
		session: null,
		pendingTotpFor: null,
		invites: [MOCK_INVITE_TOKEN],
		setupToken: MOCK_SETUP_TOKEN,
		require2fa: false,
		failedLogins: 0,
		lockedUntil: 0,
	}
}

// Lo guardado completa la semilla campo por campo: un e2e siembra solo lo que le importa
// (`e2e/mock.ts`, una instalación sin usuarios con `{ users: [] }`).
let state: AuthState = { ...seed(), ...loadPersisted<Partial<AuthState>>(STORAGE_KEY) }

export const authDb = {
	get state(): AuthState {
		return state
	},

	/** Vuelve al estado inicial; los tests lo llaman en `beforeEach`. */
	reset(overrides: Partial<AuthState> = {}): void {
		state = { ...seed(), ...overrides }
		savePersisted(STORAGE_KEY, state)
	},

	/** Guarda los cambios hechos en `state` (en el navegador sobreviven a un F5). */
	commit(): void {
		savePersisted(STORAGE_KEY, state)
	},

	findUser(username: string): MockUser | undefined {
		return state.users.find((user) => user.username === username)
	},

	/** El usuario de la sesión completa, o undefined si no hay (o espera el código). */
	currentUser(): MockUser | undefined {
		const session = state.session
		if (!session || session.mfaPending) return undefined
		return this.findUser(session.username)
	},

	needsTotpSetup(user: MockUser): boolean {
		return state.require2fa && !user.totpEnabled
	},

	addInvite(token: string): void {
		state.invites = [...state.invites, token]
		this.commit()
	},
}
