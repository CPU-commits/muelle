<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Button, Field, firstError, Input } from '@/shared/ui'
	import { PASSWORD_MIN_LENGTH } from '../../../domain'
	import {
		type AccountFormValues,
		AccountFormViewModel,
		type AccountServerErrors,
		NO_SERVER_ERRORS,
		validateNewPassword,
		validateNewUsername,
		validatePasswordRepeat,
	} from './AccountForm.viewModel.svelte.ts'
	import { fields, form as formClass } from './styles.css'

	// Usuario + contraseña + repetirla, para el setup inicial y para aceptar una invitación.
	type Props = {
		submitLabel: string
		labelledBy?: string
		disabled?: boolean
		loading?: boolean
		serverErrors?: AccountServerErrors
		onSubmit: (values: AccountFormValues) => Promise<void> | void
	}

	let {
		submitLabel,
		labelledBy,
		disabled = false,
		loading = false,
		serverErrors = NO_SERVER_ERRORS,
		onSubmit,
	}: Props = $props()

	const vm = new AccountFormViewModel({ onSubmit: (values) => onSubmit(values) })
</script>

<form
	class={formClass}
	aria-labelledby={labelledBy}
	novalidate
	onsubmit={(event) => {
		event.preventDefault()
		event.stopPropagation()
		void vm.form.handleSubmit()
	}}
>
	<div class={fields}>
		<vm.form.Field
			name="username"
			validators={{
				onBlur: ({ value }) => validateNewUsername(value),
				onSubmit: ({ value }) => validateNewUsername(value),
			}}
		>
			{#snippet children(field)}
				<Field
					label={m.auth_username()}
					hint={m.auth_username_hint()}
					error={firstError(field.state.meta.errors) ?? serverErrors.username}
				>
					{#snippet children(a11y)}
						<Input
							{...a11y}
							inputSize="lg"
							name={field.name}
							autocomplete="username"
							autocapitalize="none"
							spellcheck={false}
							value={field.state.value}
							{disabled}
							onblur={field.handleBlur}
							oninput={(event) => field.handleChange(event.currentTarget.value)}
						/>
					{/snippet}
				</Field>
			{/snippet}
		</vm.form.Field>

		<vm.form.Field
			name="password"
			validators={{
				onBlur: ({ value }) => validateNewPassword(value),
				onSubmit: ({ value }) => validateNewPassword(value),
			}}
		>
			{#snippet children(field)}
				<Field
					label={m.auth_password()}
					hint={m.auth_password_hint({ min: PASSWORD_MIN_LENGTH })}
					error={firstError(field.state.meta.errors) ?? serverErrors.password}
				>
					{#snippet children(a11y)}
						<Input
							{...a11y}
							inputSize="lg"
							type="password"
							name={field.name}
							autocomplete="new-password"
							value={field.state.value}
							{disabled}
							onblur={field.handleBlur}
							oninput={(event) => field.handleChange(event.currentTarget.value)}
						/>
					{/snippet}
				</Field>
			{/snippet}
		</vm.form.Field>

		<vm.form.Field
			name="passwordRepeat"
			validators={{
				onChangeListenTo: ['password'],
				onBlur: ({ value, fieldApi }) =>
					validatePasswordRepeat(value, fieldApi.form.getFieldValue('password')),
				onSubmit: ({ value, fieldApi }) =>
					validatePasswordRepeat(value, fieldApi.form.getFieldValue('password')),
			}}
		>
			{#snippet children(field)}
				<Field label={m.auth_password_repeat()} error={firstError(field.state.meta.errors)}>
					{#snippet children(a11y)}
						<Input
							{...a11y}
							inputSize="lg"
							type="password"
							name={field.name}
							autocomplete="new-password"
							value={field.state.value}
							{disabled}
							onblur={field.handleBlur}
							oninput={(event) => field.handleChange(event.currentTarget.value)}
						/>
					{/snippet}
				</Field>
			{/snippet}
		</vm.form.Field>
	</div>

	<Button type="submit" size="lg" fullWidth {loading} {disabled}>{submitLabel}</Button>
</form>
