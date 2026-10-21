package service

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// El log de un deploy (contrato §2.5) y las líneas de los logs de las apps
// (contrato §2.9).
const (
	LogCap       = 256 << 10 // 262 144 bytes (contrato §2.5)
	LogReserve   = 4 << 10   // para las líneas de cierre
	MaxLineBytes = 8 << 10   // una línea, sin el \n
	AppLineBytes = 16 << 10  // una línea de §2.9
	JoinMax      = 64 << 10  // los pedazos de una línea larga, unidos
	// ErrorRunes es el largo máximo de deploys.error (contrato §2.1).
	ErrorRunes      = 500
	TruncatedMarker = "… (log truncado: se superaron los 256 KB)"
)

// timeLayout es la hora de una línea: UTC con tres decimales siempre.
const timeLayout = "2006-01-02T15:04:05.000Z"

// ellipsis termina una línea cortada.
const ellipsis = "…"

// FormatLine arma "<hora> <texto>\n" (una sola línea, contrato §2.5): hora UTC
// con tres decimales (2006-01-02T15:04:05.000Z), y el texto, en este orden,
// enmascarado (Mask), limpio (Sanitize) y cortado con "…" (CutLine) para que
// la línea entera, sin el \n, mida a lo más MaxLineBytes. Se enmascara antes
// de limpiar: un secreto de varias líneas cuyas líneas miden menos de 8 bytes
// solo se reconoce entero, con sus \n; y antes de cortar, así un corte no deja
// la mitad de un secreto a la vista.
func FormatLine(m Masker, t time.Time, text string) string {
	prefix := t.UTC().Format(timeLayout) + " "
	return prefix + CutLine(Sanitize(m.Mask(text)), MaxLineBytes-len(prefix)) + "\n"
}

// Sanitize deja el texto en una línea: quita los \r y cambia por un espacio
// los \n y los demás caracteres de control (C0, DEL y C1), salvo \t. Algunos
// textos que llegan al log traen \n (los errores de compose-go); sin
// esto quedarían líneas sin hora, y un \r cortaría el evento del SSE. Un byte
// de UTF-8 inválido sale como U+FFFD.
func Sanitize(s string) string {
	clean := true
	for _, r := range s {
		if r == utf8.RuneError || (r != '\t' && unicode.IsControl(r)) {
			clean = false
			break
		}
	}
	if clean {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\r':
		case r == '\t':
			b.WriteRune(r)
		case unicode.IsControl(r):
			b.WriteByte(' ')
		default:
			b.WriteRune(r) // un byte inválido llega como utf8.RuneError (U+FFFD)
		}
	}
	return b.String()
}

// CutLine corta s a n bytes, terminando en "…" y sin partir una runa; los bytes
// de UTF-8 inválido salen como U+FFFD.
func CutLine(s string, n int) string {
	if !utf8.ValidString(s) {
		var b strings.Builder
		for _, r := range s {
			b.WriteRune(r)
		}
		s = b.String()
	}
	if len(s) <= n {
		return s
	}
	keep := n - len(ellipsis)
	if keep < 0 {
		return ""
	}
	return cutBytes(s, keep) + ellipsis
}

// AppendPlan decide qué se agrega a un log de cur bytes (contrato §2.5). lines
// ya están formateadas; marker es TruncatedMarker formateada. Las normales
// entran mientras cur+len ≤ LogCap−LogReserve; la primera que no entra agrega
// marker (si truncated era false y cabe en LogCap) y las siguientes se
// descartan. Las de cierre
// (final) entran mientras cur+len ≤ LogCap; si una no entra, ni ella ni las
// que siguen se escriben, y el log queda truncado.
func AppendPlan(cur int, truncated bool, lines []string, final bool, marker string) (add string, nowTruncated bool) {
	var b strings.Builder
	limit := LogCap - LogReserve
	if final {
		limit = LogCap
	}
	for _, line := range lines {
		if !final && truncated {
			break
		}
		if cur+b.Len()+len(line) <= limit {
			b.WriteString(line)
			continue
		}
		// La marca va en la reserva, si queda lugar (las de cierre pueden
		// haberla usado antes): el log nunca pasa de LogCap.
		if !final && cur+b.Len()+len(marker) <= LogCap {
			b.WriteString(marker)
		}
		truncated = true
		break
	}
	return b.String(), truncated
}

// JoinPieces une a la línea anterior del mismo stream cada pedazo (Time cero y
// TaskID vacío) que entrega el cliente de Docker, hasta JoinMax bytes; lo que
// pasa de ahí se descarta (contrato §2.5). Un pedazo sin línea anterior queda
// solo. No modifica lines.
func JoinPieces(lines []LogLine) []LogLine {
	out := make([]LogLine, 0, len(lines))
	last := map[string]int{} // stream → índice en out de su última línea
	for _, l := range lines {
		if l.Time.IsZero() && l.TaskID == "" {
			if i, ok := last[l.Stream]; ok {
				if room := JoinMax - len(out[i].Text); room > 0 {
					out[i].Text += cutBytes(l.Text, room)
				}
				continue
			}
		}
		last[l.Stream] = len(out)
		out = append(out, l)
	}
	return out
}

// cutBytes es s hasta n bytes, sin partir una runa.
func cutBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// SplitOutput parte la salida del CLI en líneas (sin el \r de un \r\n), sin
// las vacías del final.
func SplitOutput(out string) []string {
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return nil
	}
	return lines
}

// ErrorLine es el error de un deploy (contrato §2.1): enmascarado, después
// Sanitize (sin saltos de línea: van como espacios), y hasta ErrorRunes runas,
// terminando en "…" si se cortó.
func ErrorLine(m Masker, msg string) string {
	s := Sanitize(m.Mask(msg))
	if utf8.RuneCountInString(s) <= ErrorRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:ErrorRunes-1]) + ellipsis
}
