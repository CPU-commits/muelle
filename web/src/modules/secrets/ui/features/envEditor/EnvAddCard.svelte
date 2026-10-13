<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Banner, Button, Checkbox, Input, Textarea } from '@/shared/ui'
	import { autoGrow } from '../../components/autoGrow'
	import type { EnvEditorViewModel } from './EnvEditor.viewModel.svelte.ts'
	import {
		addCard,
		addForm,
		addTitle,
		fieldError,
		keyField,
		note,
		secretToggle,
		submitSlot,
		valueField,
		valueInput,
	} from './styles.css'

	// "Agregar variable": clave, valor y si es secreta (por defecto sí, como el backend). El
	// valor es un textarea de una línea que crece: un valor pegado de varias líneas (una
	// clave PEM) conserva sus saltos. Enter agrega; Mayús + Enter es una línea nueva.
	let { vm }: { vm: EnvEditorViewModel } = $props()

	const id = $props.id()

	// Los ids que describen cada control: su error y el error del formulario.
	const describedBy = (fieldErrorShown: boolean, fieldId: string): string | undefined =>
		[fieldErrorShown ? fieldId : null, vm.hasNewFormError ? `${id}-form-error` : null]
			.filter(Boolean)
			.join(' ') || undefined
</script>

<section class={addCard} aria-labelledby={`${id}-title`}>
	<h2 id={`${id}-title`} class={addTitle}>{m.secrets_add_title()}</h2>
	<form
		class={addForm}
		novalidate
		onsubmit={(event) => {
			event.preventDefault()
			void vm.submitAdd()
		}}
	>
		<div class={keyField}>
			<Input
				mono
				name="key"
				aria-label={m.secrets_add_key()}
				aria-invalid={vm.newKeyError !== null || vm.hasNewFormError}
				aria-describedby={describedBy(vm.newKeyError !== null, `${id}-key-error`)}
				placeholder={m.secrets_add_key_placeholder()}
				autocomplete="off"
				autocapitalize="characters"
				spellcheck={false}
				value={vm.newKey}
				oninput={(event) => vm.setNewKey(event.currentTarget.value)}
				onblur={vm.checkNewKey}
			/>
			{#if vm.newKeyError}
				<p id={`${id}-key-error`} class={fieldError} role="alert">{vm.newKeyError}</p>
			{/if}
		</div>
		<div class={valueField}>
			<Textarea
				class={valueInput}
				mono
				rows={1}
				name="value"
				aria-label={m.secrets_add_value()}
				aria-invalid={vm.newValueError !== null || vm.hasNewFormError}
				aria-describedby={describedBy(vm.newValueError !== null, `${id}-value-error`)}
				placeholder={m.secrets_add_value_placeholder()}
				autocomplete="off"
				spellcheck={false}
				value={vm.newValue}
				oninput={(event) => vm.setNewValue(event.currentTarget.value)}
				onkeydown={(event) => {
					if (event.key !== 'Enter' || event.shiftKey || event.isComposing) return
					event.preventDefault()
					event.currentTarget.form?.requestSubmit()
				}}
				{@attach autoGrow(() => vm.newValue)}
			/>
			{#if vm.newValueError}
				<p id={`${id}-value-error`} class={fieldError} role="alert">
					{vm.newValueError}
				</p>
			{/if}
		</div>
		<Checkbox
			class={secretToggle}
			label={m.secrets_add_secret()}
			checked={vm.newSecret}
			onchange={(event) => vm.setNewSecret(event.currentTarget.checked)}
		/>
		<div class={submitSlot}>
			<Button
				type="submit"
				loading={vm.isAdding}
				disabled={!vm.hasLoaded || (vm.isSaving && !vm.isAdding)}
			>
				{m.secrets_add_submit()}
			</Button>
		</div>
	</form>
	{#if vm.newFormError}
		<div id={`${id}-form-error`}>
			<Banner tone="error" title={vm.newFormError} />
		</div>
	{/if}
	<p class={note}>{m.secrets_add_note()}</p>
</section>
