import { createForm, revalidateLogic } from '@tanstack/svelte-form'
import { createMutation, useQueryClient } from '@tanstack/svelte-query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toast } from '@/shared/ui/stores'
import { createStageMutation } from '../../../data'
import type { ProjectSlug } from '../../../domain'
import { type DialogErrors, dialogErrors, NO_DIALOG_ERRORS, withoutField } from './dialogForm'

// "Nuevo stage" de un proyecto: solo el slug (staging, production…).
export class NewStageViewModel {
	errors = $state.raw<DialogErrors>(NO_DIALOG_ERRORS)

	readonly #project: () => ProjectSlug
	readonly #onDone: () => void
	readonly #client = useQueryClient()
	readonly #create = createMutation(() => createStageMutation(this.#client))

	readonly form = createForm(() => ({
		defaultValues: { slug: '' },
		// Valida al enviar y, después, en cada cambio: un error que desaparece al salir del
		// campo movería el botón justo cuando se lo clickea.
		validationLogic: revalidateLogic(),
		onSubmit: async ({ value }) => {
			await this.#submit(value.slug)
		},
	}))

	constructor(project: () => ProjectSlug, onDone: () => void) {
		this.#project = project
		this.#onDone = onDone
	}

	get project(): ProjectSlug {
		return this.#project()
	}

	get isPending(): boolean {
		return this.#create.isPending
	}

	slugEdited = (): void => {
		this.errors = withoutField(this.errors, 'slug')
	}

	async #submit(slug: string): Promise<void> {
		this.errors = NO_DIALOG_ERRORS
		try {
			const stage = await this.#create.mutateAsync({ project: this.#project(), slug })
			toast.success(m.catalog_stage_created({ stage: stage.slug }))
			this.#onDone()
		} catch (error) {
			this.errors = dialogErrors(error, ['slug'])
		}
	}
}
