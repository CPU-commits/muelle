import type {
	AuthRepository,
	InviteToken,
	LoginCredentials,
	LoginOutcome,
	Me,
	NewAccount,
	TotpCode,
	TotpSetup,
	Username,
} from '../..'
import {
	AlreadySetUp,
	InvalidCode,
	InvalidCredentials,
	InvalidInvite,
	MfaRequired,
	NoMfaPending,
	NoPendingTotp,
	SessionExpired,
	UsernameTaken,
} from '../..'

export interface FakeUser {
	username: string
	password: string
	totpEnabled: boolean
}

interface FakeSession {
	username: string
	mfaPending: boolean
	needsTotpSetup: boolean
}

export interface FakeAuthSeed {
	users?: FakeUser[]
	setupToken?: string
	invites?: string[]
	/** El único código de 2FA que este fake da por bueno. */
	validCode?: string
	/** Con `true`, las cuentas sin 2FA quedan con `needsTotpSetup`. */
	require2fa?: boolean
}

export const FAKE_VALID_CODE = '123456'
export const FAKE_TOTP_SETUP: TotpSetup = {
	secret: 'JBSWY3DPEHPK3PXP',
	otpauthUrl: 'otpauth://totp/muelle:felipe?secret=JBSWY3DPEHPK3PXP&issuer=muelle',
}

export function fakeMe(overrides: Partial<Me> = {}): Me {
	return {
		username: 'felipe' as Username,
		totpEnabled: false,
		needsTotpSetup: false,
		...overrides,
	}
}

// Un backend de identity en memoria con las mismas reglas que importan al panel: login
// en dos pasos, setup una sola vez, invitaciones de un uso y 2FA pendiente → activa.
export class InMemoryAuthRepository implements AuthRepository {
	readonly users: FakeUser[]
	readonly invites: Set<string>
	readonly setupToken: string
	readonly validCode: string
	readonly require2fa: boolean
	session: FakeSession | null = null
	pendingTotp = false
	readonly calls = {
		me: 0,
		login: 0,
		completeTotp: 0,
		logout: 0,
		setup: 0,
		acceptInvite: 0,
		beginTotpSetup: 0,
		enableTotp: 0,
	}

	constructor(seed: FakeAuthSeed = {}) {
		this.users = [...(seed.users ?? [])].map((user) => ({ ...user }))
		this.invites = new Set(seed.invites ?? [])
		this.setupToken = seed.setupToken ?? 'setup-token'
		this.validCode = seed.validCode ?? FAKE_VALID_CODE
		this.require2fa = seed.require2fa ?? false
	}

	#user(username: string): FakeUser | undefined {
		return this.users.find((user) => user.username === username)
	}

	#open(user: FakeUser, mfaPending: boolean): void {
		this.session = {
			username: user.username,
			mfaPending,
			needsTotpSetup: this.require2fa && !user.totpEnabled,
		}
	}

	me(): Promise<Me> {
		this.calls.me += 1
		const session = this.session
		if (!session) return Promise.reject(new SessionExpired())
		if (session.mfaPending) return Promise.reject(new MfaRequired())
		const user = this.#user(session.username)
		if (!user) return Promise.reject(new SessionExpired())
		return Promise.resolve({
			username: user.username as Username,
			totpEnabled: user.totpEnabled,
			needsTotpSetup: session.needsTotpSetup,
		})
	}

	login(credentials: LoginCredentials): Promise<LoginOutcome> {
		this.calls.login += 1
		const user = this.#user(credentials.username)
		if (!user || user.password !== credentials.password) {
			return Promise.reject(new InvalidCredentials())
		}
		this.#open(user, user.totpEnabled)
		return Promise.resolve({ mfaRequired: user.totpEnabled })
	}

	completeTotp(code: TotpCode): Promise<void> {
		this.calls.completeTotp += 1
		const session = this.session
		if (!session) return Promise.reject(new SessionExpired())
		if (!session.mfaPending) return Promise.reject(new NoMfaPending())
		if (code !== this.validCode) return Promise.reject(new InvalidCode())
		this.session = { ...session, mfaPending: false }
		return Promise.resolve()
	}

	logout(): Promise<void> {
		this.calls.logout += 1
		if (!this.session) return Promise.reject(new SessionExpired())
		this.session = null
		return Promise.resolve()
	}

	#create(account: NewAccount): FakeUser {
		const user: FakeUser = {
			username: account.username,
			password: account.password,
			totpEnabled: false,
		}
		this.users.push(user)
		this.#open(user, false)
		return user
	}

	setup(token: InviteToken, account: NewAccount): Promise<Username> {
		this.calls.setup += 1
		if (this.users.length > 0) return Promise.reject(new AlreadySetUp())
		if (token !== this.setupToken) return Promise.reject(new InvalidInvite())
		return Promise.resolve(this.#create(account).username as Username)
	}

	acceptInvite(token: InviteToken, account: NewAccount): Promise<Username> {
		this.calls.acceptInvite += 1
		if (!this.invites.has(token)) return Promise.reject(new InvalidInvite())
		if (this.#user(account.username)) return Promise.reject(new UsernameTaken())
		this.invites.delete(token)
		return Promise.resolve(this.#create(account).username as Username)
	}

	beginTotpSetup(): Promise<TotpSetup> {
		this.calls.beginTotpSetup += 1
		if (!this.session || this.session.mfaPending) return Promise.reject(new SessionExpired())
		this.pendingTotp = true
		return Promise.resolve(FAKE_TOTP_SETUP)
	}

	enableTotp(code: TotpCode): Promise<void> {
		this.calls.enableTotp += 1
		const session = this.session
		if (!session || session.mfaPending) return Promise.reject(new SessionExpired())
		if (!this.pendingTotp) return Promise.reject(new NoPendingTotp())
		if (code !== this.validCode) return Promise.reject(new InvalidCode())
		const user = this.#user(session.username)
		if (user) user.totpEnabled = true
		this.pendingTotp = false
		this.session = { ...session, needsTotpSetup: false }
		return Promise.resolve()
	}
}
