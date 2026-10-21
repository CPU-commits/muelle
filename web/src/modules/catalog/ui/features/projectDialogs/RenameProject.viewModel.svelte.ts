import { createForm, revalidateLogic } from '@tanstack/svelte-form'
import { createMutation, useQueryClient } from '@tanstack/svelte-query'
import { untrack } from 'svelte'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toast } from '@/shared/ui/stores'
import { renameProjectMutation } from '../../../data'
import type { Project } from '../../../domain'
import {
	type DialogErrors,
	dialogErrors,
	formErrors,
	NO_DIALOG_ERRORS,
	validateProjectName,
	withoutField,
} from './dialogForm'

// "Renombrar": solo el nombre visible; el slug no cambia.
export class RenameProjectViewModel {
	errors = $state.raw<DialogErrors>(NO_DIALOG_ERRORS)

	readonly #project: () => Project
	readonly #onDone: () => void
	readonly #client = useQueryClient()
	readonly #rename = createMutation(() => renameProjectMutation(this.#client))
	readonly form

	constructor(project: () => Project, onDone: () => void) {
		this.#project = project
		this.#onDone = onDone
		this.form = createForm(() => ({
			defaultValues: { name: untrack(() => this.#project().name) },
			// Valida al enviar y, después, en cada cambio: un error que desaparece al salir del
			// campo movería el botón justo cuando se lo clickea.
			validationLogic: revalidateLogic(),
			validators: {
				onDynamic: ({ value }: { value: { name: string } }) =>
					formErrors({ name: validateProjectName(value.name) }),
			},
			onSubmit: async ({ value }) => {
				await this.#submit(value.name)
			},
		}))
	}

	get slug(): string {
		return this.#project().slug
	}

	get isPending(): boolean {
		return this.#rename.isPending
	}

	nameEdited = (): void => {
		this.errors = withoutField(this.errors, 'name')
	}

	async #submit(name: string): Promise<void> {
		this.errors = NO_DIALOG_ERRORS
		// El mismo nombre: no hay nada que cambiar (ni que auditar).
		if (name.trim() === this.#project().name) {
			this.#onDone()
			return
		}
		try {
			await this.#rename.mutateAsync({ project: this.#project().slug, name })
			toast.success(m.catalog_project_renamed())
			this.#onDone()
		} catch (error) {
			this.errors = dialogErrors(error, ['name'])
		}
	}
}
