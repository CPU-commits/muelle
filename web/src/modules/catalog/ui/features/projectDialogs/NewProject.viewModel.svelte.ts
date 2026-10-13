import { createForm, revalidateLogic } from '@tanstack/svelte-form'
import { createMutation, useQueryClient } from '@tanstack/svelte-query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toast } from '@/shared/ui/stores'
import { createProjectMutation } from '../../../data'
import { PROJECT_SLUG_MAX, slugFromName } from '../../../domain'
import {
	type DialogErrors,
	dialogErrors,
	formErrors,
	NO_DIALOG_ERRORS,
	validateProjectName,
	validateProjectSlug,
	withoutField,
} from './dialogForm'

export interface NewProjectValues {
	name: string
	slug: string
}

// "Nuevo proyecto": nombre y slug. El slug se sugiere a partir del nombre hasta que la
// persona lo edita.
export class NewProjectViewModel {
	errors = $state.raw<DialogErrors>(NO_DIALOG_ERRORS)
	#slugEdited = false

	readonly #onDone: () => void
	readonly #client = useQueryClient()
	readonly #create = createMutation(() => createProjectMutation(this.#client))

	readonly form = createForm(() => ({
		defaultValues: { name: '', slug: '' } as NewProjectValues,
		// Valida al enviar y, después, en cada cambio: un error que desaparece al salir del
		// campo movería el botón justo cuando se lo clickea.
		validationLogic: revalidateLogic(),
		validators: {
			onDynamic: ({ value }: { value: NewProjectValues }) =>
				formErrors({
					name: validateProjectName(value.name),
					slug: validateProjectSlug(value.slug),
				}),
		},
		onSubmit: async ({ value }) => {
			await this.#submit(value)
		},
	}))

	constructor(onDone: () => void) {
		this.#onDone = onDone
	}

	get isPending(): boolean {
		return this.#create.isPending
	}

	suggestSlug = (name: string): void => {
		if (!this.#slugEdited) {
			this.form.setFieldValue('slug', slugFromName(name, PROJECT_SLUG_MAX), {
				dontUpdateMeta: true,
			})
		}
	}

	slugEdited = (): void => {
		this.#slugEdited = true
		this.errors = withoutField(this.errors, 'slug')
	}

	nameEdited = (name: string): void => {
		this.errors = withoutField(this.errors, 'name')
		this.suggestSlug(name)
	}

	async #submit(values: NewProjectValues): Promise<void> {
		this.errors = NO_DIALOG_ERRORS
		try {
			const project = await this.#create.mutateAsync(values)
			toast.success(m.catalog_project_created({ project: project.slug }))
			this.#onDone()
		} catch (error) {
			this.errors = dialogErrors(error, ['name', 'slug'])
		}
	}
}
