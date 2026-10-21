import { goto } from '$app/navigation'
import { resolve } from '$app/paths'
import { createMutation, useQueryClient } from '@tanstack/svelte-query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toast } from '@/shared/ui/stores'
import { deleteAppMutation } from '../../../data'
import { AppNotFound, type AppRef, appStackName } from '../../../domain'
import { catalogErrorMessage } from '../../components/catalogMessages'

// "Eliminar app" al pie de Configuración: confirma, borra y vuelve a Proyectos.
export class DeleteAppViewModel {
	confirming = $state(false)

	readonly #ref: () => AppRef
	readonly #client = useQueryClient()
	readonly #delete = createMutation(() => deleteAppMutation(this.#client))

	constructor(ref: () => AppRef) {
		this.#ref = ref
	}

	get app(): string {
		return this.#ref().app
	}

	get stackName(): string {
		return appStackName(this.#ref())
	}

	get isPending(): boolean {
		return this.#delete.isPending
	}

	ask = (): void => {
		this.confirming = true
	}

	cancel = (): void => {
		if (!this.isPending) this.confirming = false
	}

	confirm = async (): Promise<void> => {
		const ref = this.#ref()
		try {
			await this.#delete.mutateAsync(ref)
		} catch (error) {
			// Si ya no existe, el resultado es el mismo: no está.
			if (!(error instanceof AppNotFound)) {
				this.confirming = false
				toast.error(m.catalog_delete_app_error(), catalogErrorMessage(error))
				return
			}
		}
		this.confirming = false
		toast.success(m.catalog_app_deleted({ app: ref.app }))
		await goto(resolve('/'))
	}
}
