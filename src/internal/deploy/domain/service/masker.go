package service

import (
	"cmp"
	"slices"
	"strings"
)

// minMaskBytes es el largo mínimo de un valor (o de una línea de un valor)
// que se enmascara: uno más corto (1, true, 3000) taparía texto que no tiene
// nada que ver (contrato §2.5).
const minMaskBytes = 8

// Masker reemplaza los valores secretos por *** (contrato §2.5): cada valor
// de 8 bytes o más y, de uno de varias líneas, también cada línea (sin los
// espacios de los bordes) de 8 bytes o más; sin repetidos y del más largo al
// más corto, uno después del otro (strings.ReplaceAll en ese orden). El cero
// no enmascara nada.
type Masker struct{ values []string }

// NewMasker arma el Masker con los valores secretos de ResolveEnv.
func NewMasker(secretValues []string) Masker {
	seen := map[string]bool{}
	var values []string
	add := func(v string) {
		if len(v) >= minMaskBytes && !seen[v] {
			seen[v] = true
			values = append(values, v)
		}
	}
	for _, v := range secretValues {
		add(v)
		if strings.ContainsAny(v, "\n\r") {
			for _, line := range strings.FieldsFunc(v, isLineBreak) {
				add(strings.TrimSpace(line))
			}
		}
	}
	// Del más largo al más corto; a igual largo, en orden, para que el
	// resultado no dependa del orden de llegada.
	slices.SortFunc(values, func(a, b string) int {
		return cmp.Or(cmp.Compare(len(b), len(a)), strings.Compare(a, b))
	})
	return Masker{values: values}
}

func isLineBreak(r rune) bool { return r == '\n' || r == '\r' }

// Mask devuelve s con cada valor secreto como ***.
func (m Masker) Mask(s string) string {
	for _, v := range m.values {
		if strings.Contains(s, v) {
			s = strings.ReplaceAll(s, v, "***")
		}
	}
	return s
}
