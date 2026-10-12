import { createMutation, useQueryClient } from '@tanstack/svelte-query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toast } from '@/shared/ui/stores'
import { rollbackMutation } from '../../../data'
import {
	type AppRef,
	type Deploy,
	type DeployId,
	imageName,
	NoPreviousDeploy,
} from '../../../domain'
import { deployErrorMessage } from '../../components/labels/deployErrorMessage'

export interface RollbackDialogProps {
	readonly app: AppRef
	/** A dónde vuelve según el historial (el succeeded anterior al actual), si se sabe. */
	readonly target: Deploy | null
	/**
	 * El historial está entero (menos de 20 deploys): si no hay `target`, el backend va a
	 * responder 409 seguro y se avisa antes de intentarlo.
	 */
	readonly historyComplete: boolean
	readonly onDone: (id: DeployId) => void
	readonly onClose: () => void
}

// "Rollback a #140" (App-Resumen.dc.html): las imágenes del deploy exitoso anterior con
// la configuración actual. Sin anterior, el backend responde 409 → `NoPreviousDeploy`.
export class RollbackDialogViewModel {
	formError = $state<string | null>(null)
	/** El backend confirmó que no hay a dónde volver. */
	noPrevious = $state(false)

	readonly #props: () => RollbackDialogProps
	readonly #client = useQueryClient()
	readonly #rollback = createMutation(() => rollbackMutation(this.#client))

	constructor(props: () => RollbackDialogProps) {
		this.#props = props
	}

	get target(): Deploy | null {
		return this.#props().target
	}

	/** Seguro que no hay a dónde volver: lo dijo el backend o el historial entero. */
	get nothingToRollBack(): boolean {
		const props = this.#props()
		return this.noPrevious || (props.target === null && props.historyComplete)
	}

	get isPending(): boolean {
		return this.#rollback.isPending
	}

	get title(): string {
		const target = this.target
		return target
			? m.deploy_rollback_title({ id: target.id })
			: m.deploy_rollback_title_generic()
	}

	/** "backend-app:sha-e52b8f0", o los servicios de una compose separados por coma. */
	get targetImages(): string {
		return (this.target?.images ?? []).map((image) => imageName(image.ref)).join(', ')
	}

	get targetDeployedAt(): Date | null {
		const target = this.target
		return target ? (target.finishedAt ?? target.createdAt) : null
	}

	close = (): void => {
		if (this.#rollback.isPending) return
		this.formError = null
		this.noPrevious = false
		this.#props().onClose()
	}

	confirm = async (): Promise<void> => {
		if (this.#rollback.isPending || this.nothingToRollBack) return
		this.formError = null
		const props = this.#props()
		try {
			const { id } = await this.#rollback.mutateAsync(props.app)
			toast.success(m.deploy_toast_rollback({ id }), m.deploy_toast_queued_body())
			props.onDone(id)
		} catch (error) {
			if (error instanceof NoPreviousDeploy) {
				this.noPrevious = true
				return
			}
			this.formError = deployErrorMessage(error)
		}
	}
}
