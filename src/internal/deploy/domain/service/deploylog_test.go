package service

import (
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

var t0 = time.Date(2026, 10, 10, 14, 32, 7, 120_000_000, time.UTC)

func TestFormatLine(t *testing.T) {
	m := NewMasker([]string{"ab\ncdefg\nhi", "s3cr3t-v4lor"})
	cases := []struct{ name, text, want string }{
		{"hora con tres decimales", "resolviendo imágenes", "2026-10-10T14:32:07.120Z resolviendo imágenes\n"},
		{"sin \\r", "uno\r dos\r", "2026-10-10T14:32:07.120Z uno dos\n"},
		// El texto de un InvalidTemplateError de compose-go trae un \n.
		{"un \\n adentro es un espacio",
			"invalid interpolation format for x.\nYou may need to escape any $ with another $.",
			"2026-10-10T14:32:07.120Z invalid interpolation format for x. You may need to escape any $ with another $.\n"},
		{"controles", "a\x1bb\x00c\x7fd\u0085e\tf", "2026-10-10T14:32:07.120Z a b c d e\tf\n"},
		// Se enmascara antes de limpiar: con los \n cambiados por espacios, ni
		// el valor entero ni sus líneas (de menos de 8 bytes) se reconocerían.
		{"secreto de varias líneas cortas", "valor=ab\ncdefg\nhi.", "2026-10-10T14:32:07.120Z valor=***.\n"},
		{"secreto de una línea", "DB=s3cr3t-v4lor", "2026-10-10T14:32:07.120Z DB=***\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := FormatLine(m, t0, c.text)
			if got != c.want {
				t.Fatalf("got %q\nesperaba %q", got, c.want)
			}
			if strings.Count(got, "\n") != 1 || !strings.HasSuffix(got, "\n") {
				t.Fatalf("debe ser una sola línea: %q", got)
			}
		})
	}
	// La hora va en UTC aunque llegue en otra zona.
	if got := FormatLine(Masker{}, t0.In(time.FixedZone("CLT", -3*3600)), "x"); !strings.HasPrefix(got, "2026-10-10T14:32:07.120Z ") {
		t.Errorf("%q", got)
	}
}

func TestFormatLineCut(t *testing.T) {
	// Un secreto que cruza el byte 8192 queda entero como ***: se enmascara
	// antes de cortar.
	secret := strings.Repeat("S", 100)
	text := strings.Repeat("x", 8150) + secret + strings.Repeat("y", 100)
	got := FormatLine(NewMasker([]string{secret}), t0, text)
	if strings.Contains(got, "SSSSSSSS") {
		t.Fatalf("quedó parte del secreto a la vista")
	}
	if !strings.Contains(got, "***") {
		t.Fatalf("falta el ***")
	}
	if len(got) > MaxLineBytes+1 {
		t.Fatalf("mide %d bytes", len(got))
	}

	// Corte: a lo más 8192 bytes sin el \n, UTF-8 válido con una ñ justo en el
	// corte, y termina en "…".
	prefix := len("2026-10-10T14:32:07.120Z ")
	for shift := range 4 {
		text := strings.Repeat("a", MaxLineBytes-prefix-len("…")-shift) + strings.Repeat("ñ", 50)
		got := FormatLine(Masker{}, t0, text)
		line := strings.TrimSuffix(got, "\n")
		if len(line) > MaxLineBytes || !utf8.ValidString(got) || !strings.HasSuffix(line, "…") {
			t.Fatalf("shift %d: %d bytes, válido %v, %q", shift, len(line), utf8.ValidString(got), line[len(line)-10:])
		}
	}
	// Una que cabe justo no se corta.
	exact := strings.Repeat("a", MaxLineBytes-prefix)
	if got := FormatLine(Masker{}, t0, exact); strings.Contains(got, "…") || len(got) != MaxLineBytes+1 {
		t.Fatalf("se cortó una línea que cabía: %d bytes", len(got))
	}
}

func TestSanitize(t *testing.T) {
	for in, want := range map[string]string{
		"hola":                     "hola",
		"a\r\nb":                   "a b",
		"a\nb\rc":                  "a bc",
		"tab\tok":                  "tab\tok",
		"\x01\x1f\x7f\u0080\u009f": "     ",
		"ñandú   x":                "ñandú   x",
		"inválido \xff fin":        "inválido � fin",
	} {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, esperaba %q", in, got, want)
		}
	}
}

func TestCutLine(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"corto", 10, "corto"},
		{"exacto", 6, "exacto"},
		{"demasiado largo", 10, "demasia…"},
		{"ññññ", 7, "ññ…"},  // 8 bytes: quedan 4 para el texto
		{"ññññ", 8, "ññññ"}, // cabe
		{"ññññ", 6, "ñ…"},   // quedan 3: la segunda ñ no se parte
		{"a\xffb", 10, "a�b"},
		{"\xff\xff\xff\xff", 7, "�…"},
	}
	for _, c := range cases {
		got := CutLine(c.in, c.n)
		if got != c.want {
			t.Errorf("CutLine(%q, %d) = %q, esperaba %q", c.in, c.n, got, c.want)
		}
		if len(got) > c.n || !utf8.ValidString(got) {
			t.Errorf("CutLine(%q, %d) = %q: %d bytes, válido %v", c.in, c.n, got, len(got), utf8.ValidString(got))
		}
	}
}

func TestAppendPlan(t *testing.T) {
	marker := FormatLine(Masker{}, t0, TruncatedMarker)
	line := FormatLine(Masker{}, t0, "una línea")
	limit := LogCap - LogReserve

	// Entra la normal que cabe.
	add, tr := AppendPlan(0, false, []string{line, line}, false, marker)
	if add != line+line || tr {
		t.Fatalf("add=%q truncated=%v", add, tr)
	}
	// Justo en el límite entra; la siguiente agrega la marca una sola vez y
	// las demás se descartan.
	cur := limit - len(line)
	add, tr = AppendPlan(cur, false, []string{line, line, line}, false, marker)
	if add != line+marker || !tr {
		t.Fatalf("add=%q truncated=%v", add, tr)
	}
	// Ya truncado: las normales se descartan, sin otra marca.
	add, tr = AppendPlan(cur+len(add), true, []string{line}, false, marker)
	if add != "" || !tr {
		t.Fatalf("add=%q truncated=%v", add, tr)
	}
	// Una de cierre entra en la reserva aunque el log esté truncado.
	closing := FormatLine(Masker{}, t0, "failed: timeout esperando convergencia")
	add, tr = AppendPlan(limit+len(marker), true, []string{closing}, true, marker)
	if add != closing || !tr {
		t.Fatalf("cierre: add=%q truncated=%v", add, tr)
	}
	// Una de cierre que pasa LogCap no entra (ni las que siguen).
	add, tr = AppendPlan(LogCap-len(closing)+1, true, []string{closing, "x\n"}, true, marker)
	if add != "" || !tr {
		t.Fatalf("cierre que no cabe: add=%q truncated=%v", add, tr)
	}
	// Una de cierre que cabe justo en LogCap entra.
	add, _ = AppendPlan(LogCap-len(closing), false, []string{closing}, true, marker)
	if add != closing {
		t.Fatalf("cierre justo: add=%q", add)
	}
	// Si las de cierre ya llenaron la reserva, la marca tampoco pasa de LogCap.
	add, tr = AppendPlan(LogCap-10, false, []string{line}, false, marker)
	if add != "" || !tr {
		t.Fatalf("marca sin lugar: add=%q truncated=%v", add, tr)
	}
	// Sin líneas no cambia nada.
	if add, tr := AppendPlan(10, false, nil, false, marker); add != "" || tr {
		t.Fatalf("add=%q truncated=%v", add, tr)
	}
}

func TestJoinPieces(t *testing.T) {
	at := func(s int) time.Time { return t0.Add(time.Duration(s) * time.Second) }
	in := []LogLine{
		{Stream: "stdout", Text: "pedazo sin línea anterior"},
		{Time: at(1), Stream: "stdout", TaskID: "t1", Text: "AAAA"},
		{Time: at(2), Stream: "stderr", TaskID: "t1", Text: "error"},
		{Stream: "stdout", Text: "BBBB"},
		{Stream: "stdout", Text: "CCCC"},
		{Stream: "stderr", Text: "-más"},
		{Time: at(3), Stream: "stdout", TaskID: "t2", Text: "otra"},
	}
	got := JoinPieces(in)
	want := []LogLine{
		{Stream: "stdout", Text: "pedazo sin línea anterior"},
		{Time: at(1), Stream: "stdout", TaskID: "t1", Text: "AAAABBBBCCCC"},
		{Time: at(2), Stream: "stderr", TaskID: "t1", Text: "error-más"},
		{Time: at(3), Stream: "stdout", TaskID: "t2", Text: "otra"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v\nesperaba %+v", got, want)
	}
	if in[1].Text != "AAAA" {
		t.Fatal("modificó la entrada")
	}

	// El tope: 64 KiB unidos; el resto se descarta.
	piece := strings.Repeat("p", 16<<10)
	long := []LogLine{{Time: at(1), Stream: "stdout", TaskID: "t1", Text: piece}}
	for range 5 {
		long = append(long, LogLine{Stream: "stdout", Text: piece})
	}
	long = append(long, LogLine{Time: at(2), Stream: "stdout", TaskID: "t1", Text: "sigue"})
	got = JoinPieces(long)
	if len(got) != 2 || len(got[0].Text) != JoinMax || got[1].Text != "sigue" {
		t.Fatalf("%d líneas, la primera de %d bytes", len(got), len(got[0].Text))
	}
	// El tope no parte una runa.
	got = JoinPieces([]LogLine{
		{Time: at(1), Stream: "stdout", TaskID: "t1", Text: strings.Repeat("a", JoinMax-1)},
		{Stream: "stdout", Text: "ñb"},
	})
	if len(got) != 1 || len(got[0].Text) != JoinMax-1 || !utf8.ValidString(got[0].Text) {
		t.Fatalf("%d bytes, válido %v", len(got[0].Text), utf8.ValidString(got[0].Text))
	}
}

func TestSplitOutput(t *testing.T) {
	cases := map[string][]string{
		"":                                       nil,
		"\n\n":                                   nil,
		"Creating service x_web\n":               {"Creating service x_web"},
		"uno\r\ndos\n\ntres\n\n\n":               {"uno", "dos", "", "tres"},
		"sin salto final":                        {"sin salto final"},
		"Updating service a\nUpdating service b": {"Updating service a", "Updating service b"},
	}
	for in, want := range cases {
		if got := SplitOutput(in); !slices.Equal(got, want) {
			t.Errorf("SplitOutput(%q) = %q, esperaba %q", in, got, want)
		}
	}
}

func TestErrorLine(t *testing.T) {
	m := NewMasker([]string{"ab\ncdefg\nhi", "s3cr3t-v4lor"})
	long := strings.Repeat("é", 600)
	got := ErrorLine(m, long)
	if n := utf8.RuneCountInString(got); n != 500 || !strings.HasSuffix(got, "…") {
		t.Fatalf("%d runas", n)
	}
	if got := ErrorLine(m, "render: x\ny\r"); got != "render: x y" {
		t.Fatalf("%q", got)
	}
	if got := ErrorLine(m, "render: compose_yaml: parsing \"s3cr3t-v4lor\": invalid syntax"); strings.Contains(got, "s3cr3t") {
		t.Fatalf("%q", got)
	}
	if got := ErrorLine(m, "valor ab\ncdefg\nhi"); got != "valor ***" {
		t.Fatalf("varias líneas: %q", got)
	}
	if got := ErrorLine(m, strings.Repeat("x", 500)); got != strings.Repeat("x", 500) {
		t.Fatal("500 runas caben")
	}
}
