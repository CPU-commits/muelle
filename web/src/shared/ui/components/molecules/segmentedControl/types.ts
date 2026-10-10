export interface ChoiceOption<T extends string> {
	readonly value: T
	readonly label: string
	readonly disabled?: boolean
}
