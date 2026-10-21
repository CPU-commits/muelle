declare const brandSymbol: unique symbol

export type Brand<T, B extends string> = T & { readonly [brandSymbol]: B }
