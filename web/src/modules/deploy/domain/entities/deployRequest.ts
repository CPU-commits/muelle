import { parseImageOverride } from '../value-objects/imageRef'

// El cuerpo de `POST …/deploys` (spec §7):
// - app de formulario: `{"image": "…"}`;
// - app compose: `{"images": {"web": "…"}}`;
// - vacío: redespliega las refs actuales y vuelve a resolver los digests.
export type DeployRequest =
	| { readonly kind: 'redeploy' }
	| { readonly kind: 'image'; readonly image: string }
	| { readonly kind: 'images'; readonly images: Readonly<Record<string, string>> }

/** El `kind` de la app en el catálogo (`service` es la de formulario). */
export type AppKind = 'service' | 'compose'

export interface DeployOverridesInput {
	/** El slug de la app: en una app de formulario, su único servicio se llama igual (§5.2). */
	readonly app: string
	/** El tipo de la app si se conoce (lo da el catálogo); si no, se deduce de `services`. */
	readonly kind?: AppKind | null
	/** Los servicios que tiene hoy (del estado). Vacío si nunca se desplegó. */
	readonly services: readonly string[]
	/** Lo que se escribió para cada servicio; vacío = la imagen actual. */
	readonly overrides: Readonly<Record<string, string>>
}

/**
 * Sin `kind`, una app con un único servicio que se llama como la app se trata como de
 * formulario; cualquier otra, como compose. Sin overrides es un redeploy. Una imagen
 * inválida es un `ValidationError` con el servicio como `field`.
 */
export function inferAppKind(app: string, services: readonly string[]): AppKind {
	if (services.length === 0) return 'service'
	return services.length === 1 && services[0] === app ? 'service' : 'compose'
}

export function buildDeployRequest(input: DeployOverridesInput): DeployRequest {
	const filled = Object.entries(input.overrides).filter(([, value]) => value.trim() !== '')
	if (filled.length === 0) return { kind: 'redeploy' }

	const images: Record<string, string> = {}
	for (const [service, value] of filled) images[service] = parseImageOverride(value, service)

	const kind = input.kind ?? inferAppKind(input.app, input.services)
	if (kind === 'service') {
		const image = images[input.app] ?? Object.values(images)[0]
		if (image !== undefined) return { kind: 'image', image }
	}
	return { kind: 'images', images }
}

/**
 * Los servicios a los que se les puede pedir una imagen: los de la configuración (los
 * `services` del catálogo, contrato §2.12), que son contra los que el backend valida
 * `images` (§2.2), aunque la app no se haya desplegado nunca. Sin ellos (un backend sin la
 * Fase 2, el catálogo que no cargó, o `[]` de un compose guardado que no se puede leer), los
 * que corren en Swarm.
 */
export function deployableServices(
	configured: readonly string[] | null | undefined,
	running: readonly string[],
): readonly string[] {
	return configured && configured.length > 0 ? configured : running
}

/** Los servicios para los que el diálogo ofrece un override (uno solo en una de formulario). */
export function overridableServices(
	app: string,
	services: readonly string[],
	kind?: AppKind | null,
): string[] {
	if (kind === 'service' || services.length === 0) return [app]
	return [...services]
}
