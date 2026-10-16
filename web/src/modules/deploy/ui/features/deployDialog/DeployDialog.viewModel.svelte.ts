import { createMutation, useQueryClient } from '@tanstack/svelte-query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toast } from '@/shared/ui/stores'
import { createDeployMutation } from '../../../data'
import {
	type AppKind,
	type AppRef,
	type DeployId,
	ImageRejected,
	isValidImageOverride,
	overridableServices,
	type RollbackPin,
} from '../../../domain'
import { deployErrorMessage } from '../../components/labels/deployErrorMessage'

export interface DeployDialogProps {
	readonly app: AppRef
	/** `service`/`compose` del catálogo si se sabe; si no, se deduce de `services`. */
	readonly kind: AppKind | null
	/** Los servicios que tiene hoy la app (del estado). */
	readonly services: readonly string[]
	/** La imagen actual de cada servicio, para el placeholder. */
	readonly currentImages: Readonly<Record<string, string>>
	/**
	 * Lo que sigue fijado por un rollback (`rollbackPin`): un deploy sin imagen repite esas
	 * refs con digest, así que no vuelve a seguir el tag (contrato §2.2). El diálogo lo avisa.
	 */
	readonly rollbackPin?: RollbackPin | null
	readonly onDeployed: (id: DeployId) => void
	readonly onClose: () => void
}

/** El texto de cada motivo por el que el backend rechazó una imagen. */
function rejectionMessage(error: ImageRejected, service: string): string {
	switch (error.reason) {
		case 'both':
			return m.deploy_dialog_reason_both()
		case 'compose':
			return m.deploy_dialog_reason_compose()
		case 'unknown_service':
			return m.deploy_dialog_reason_unknown_service({ service })
		case 'invalid':
			return m.deploy_dialog_invalid_image()
	}
}

// "Desplegar" (App-Resumen.dc.html): un campo de imagen opcional por servicio. Vacío
// repite la ref pedida del deploy actual: un tag se vuelve a resolver, un digest queda fijo
// (contrato §2.2).
export class DeployDialogViewModel {
	overrides = $state<Record<string, string>>({})
	errors = $state<Record<string, string>>({})
	formError = $state<string | null>(null)

	readonly #props: () => DeployDialogProps
	readonly #client = useQueryClient()
	readonly #deploy = createMutation(() => createDeployMutation(this.#client))

	constructor(props: () => DeployDialogProps) {
		this.#props = props
	}

	get fields(): string[] {
		const { app, services, kind } = this.#props()
		return overridableServices(app.app, services, kind)
	}

	get isPending(): boolean {
		return this.#deploy.isPending
	}

	/** El título del aviso de un rollback que sigue fijando imágenes; null si no hay. */
	get rollbackPinTitle(): string | null {
		const pin = this.#props().rollbackPin
		if (!pin) return null
		const id = pin.rollback
		return this.fields.length > 1
			? m.deploy_dialog_rollback_pin_services_title({ id, services: pin.services.join(', ') })
			: m.deploy_dialog_rollback_pin_title({ id })
	}

	placeholder(service: string): string {
		return this.#props().currentImages[service] ?? ''
	}

	setOverride = (service: string, value: string): void => {
		this.overrides = { ...this.overrides, [service]: value }
		if (this.errors[service]) {
			const rest = { ...this.errors }
			delete rest[service]
			this.errors = rest
		}
		this.formError = null
	}

	close = (): void => {
		if (this.#deploy.isPending) return
		this.#reset()
		this.#props().onClose()
	}

	submit = async (): Promise<void> => {
		if (this.#deploy.isPending) return
		this.formError = null
		const invalid: Record<string, string> = {}
		for (const [service, value] of Object.entries(this.overrides)) {
			if (value.trim() && !isValidImageOverride(value)) {
				invalid[service] = m.deploy_dialog_invalid_image()
			}
		}
		this.errors = invalid
		if (Object.keys(invalid).length > 0) return

		const props = this.#props()
		try {
			const { id } = await this.#deploy.mutateAsync({
				app: props.app,
				kind: props.kind,
				services: props.services,
				overrides: this.overrides,
			})
			toast.success(m.deploy_toast_queued({ id }), m.deploy_toast_queued_body())
			this.#reset()
			props.onDeployed(id)
		} catch (error) {
			this.#showError(error)
		}
	}

	#showError(error: unknown): void {
		if (error instanceof ImageRejected) {
			// El backend dice `image` (formulario) o `images.<servicio>` (compose).
			const service = error.service ?? this.fields[0]
			const message = rejectionMessage(error, service ?? '')
			if (service && this.fields.includes(service)) {
				this.errors = { ...this.errors, [service]: message }
			} else {
				this.formError = message
			}
			return
		}
		this.formError = deployErrorMessage(error)
	}

	#reset(): void {
		this.overrides = {}
		this.errors = {}
		this.formError = null
	}
}
