import { goto } from '$app/navigation'
import { resolve } from '$app/paths'
import { createForm, revalidateLogic } from '@tanstack/svelte-form'
import { createMutation, useQueryClient } from '@tanstack/svelte-query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toast } from '@/shared/ui/stores'
import { createAppMutation } from '../../../data'
import {
	type AppKind,
	type CreateAppInput,
	newServiceSpec,
	type ProjectSlug,
	type StageSlug,
} from '../../../domain'
import {
	type DialogErrors,
	dialogErrors,
	formErrors,
	NO_DIALOG_ERRORS,
	validateAppSlug,
	validateComposeYaml,
	validateImage,
	validatePort,
	withoutField,
} from './dialogForm'

export interface NewAppValues {
	slug: string
	kind: AppKind
	image: string
	port: string
	composeYaml: string
}

/** Un punto de partida para una app compose: se edita entero después. */
export const COMPOSE_STARTER = `services:
  web:
    image: nginx:1.27-alpine
`

const DEFAULTS: NewAppValues = {
	slug: '',
	kind: 'service',
	image: '',
	port: '',
	composeYaml: COMPOSE_STARTER,
}

// "Nueva app en proyecto/stage": de formulario (imagen y puerto, el resto con los valores
// del backend) o compose (el YAML). Se crea sin desplegar y lleva a su Configuración.
export class NewAppViewModel {
	errors = $state.raw<DialogErrors>(NO_DIALOG_ERRORS)

	readonly #where: () => { project: ProjectSlug; stage: StageSlug }
	readonly #onDone: () => void
	readonly #client = useQueryClient()
	readonly #create = createMutation(() => createAppMutation(this.#client))
	readonly #kind

	readonly form = createForm(() => ({
		defaultValues: DEFAULTS,
		// Valida al enviar y, después, en cada cambio: un error que desaparece al salir del
		// campo movería el botón justo cuando se lo clickea.
		validationLogic: revalidateLogic(),
		// Solo lo del tipo elegido: la imagen de una compose no importa.
		validators: {
			onDynamic: ({ value }: { value: NewAppValues }) =>
				formErrors(
					value.kind === 'service'
						? {
								slug: validateAppSlug(value.slug),
								image: validateImage(value.image),
								port: validatePort(value.port),
							}
						: {
								slug: validateAppSlug(value.slug),
								composeYaml: validateComposeYaml(value.composeYaml),
							},
				),
		},
		onSubmit: async ({ value }) => {
			await this.#submit(value)
		},
	}))

	constructor(where: () => { project: ProjectSlug; stage: StageSlug }, onDone: () => void) {
		this.#where = where
		this.#onDone = onDone
		this.#kind = this.form.useSelector((state) => state.values.kind)
	}

	readonly kindOptions = [
		{ value: 'service', label: m.catalog_app_kind_service() },
		{ value: 'compose', label: m.catalog_app_kind_compose() },
	] as const satisfies readonly { value: AppKind; label: string }[]

	get project(): ProjectSlug {
		return this.#where().project
	}

	get stage(): StageSlug {
		return this.#where().stage
	}

	get kind(): AppKind {
		return this.#kind.current
	}

	get kindHint(): string {
		return this.kind === 'service'
			? m.catalog_app_kind_service_hint()
			: m.catalog_app_kind_compose_hint()
	}

	get isPending(): boolean {
		return this.#create.isPending
	}

	setKind = (kind: AppKind): void => {
		this.form.setFieldValue('kind', kind)
		this.errors = NO_DIALOG_ERRORS
	}

	edited = (field: keyof NewAppValues): void => {
		this.errors = withoutField(this.errors, field)
	}

	#input(values: NewAppValues): CreateAppInput {
		const where = { ...this.#where(), slug: values.slug }
		return values.kind === 'service'
			? {
					...where,
					kind: 'service',
					spec: newServiceSpec(values.image.trim(), Number(values.port)),
				}
			: { ...where, kind: 'compose', composeYaml: values.composeYaml }
	}

	async #submit(values: NewAppValues): Promise<void> {
		this.errors = NO_DIALOG_ERRORS
		try {
			const app = await this.#create.mutateAsync(this.#input(values))
			toast.success(m.catalog_app_created({ app: app.slug }), m.catalog_app_created_body())
			this.#onDone()
			await goto(
				resolve('/(app)/projects/[p]/stages/[s]/apps/[a]/configuration', {
					p: app.project,
					s: app.stage,
					a: app.slug,
				}),
			)
		} catch (error) {
			this.errors = dialogErrors(error, ['slug', 'image', 'port', 'composeYaml'])
		}
	}
}
