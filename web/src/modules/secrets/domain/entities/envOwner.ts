// El dueño de un set de variables: un stage (las reciben todas sus apps) o una app (las
// suyas pisan a las del stage). Se identifica por slugs, como en las rutas de la API.
export type EnvOwner =
	| { readonly kind: 'stage'; readonly project: string; readonly stage: string }
	| {
			readonly kind: 'app'
			readonly project: string
			readonly stage: string
			readonly app: string
	  }

export type EnvOwnerKind = EnvOwner['kind']

export function stageOwner(project: string, stage: string): EnvOwner {
	return { kind: 'stage', project, stage }
}

export function appOwner(project: string, stage: string, app: string): EnvOwner {
	return { kind: 'app', project, stage, app }
}

/** El stage de un dueño: el propio, o el de la app. */
export function ownerStage(owner: EnvOwner): EnvOwner {
	return stageOwner(owner.project, owner.stage)
}

export function sameOwner(a: EnvOwner, b: EnvOwner): boolean {
	if (a.project !== b.project || a.stage !== b.stage) return false
	if (a.kind === 'stage' || b.kind === 'stage') return a.kind === b.kind
	return a.app === b.app
}

/** `stage:<p>/<s>` o `app:<p>/<s>/<a>`, como el target de la auditoría del backend. */
export function ownerLabel(owner: EnvOwner): string {
	return owner.kind === 'stage'
		? `stage:${owner.project}/${owner.stage}`
		: `app:${owner.project}/${owner.stage}/${owner.app}`
}
