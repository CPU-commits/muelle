package compose

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/template"
	"github.com/sirupsen/logrus"
)

func mapping(m map[string]string) template.Mapping {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func TestStrictSubstitute(t *testing.T) {
	vars := map[string]string{"A": "a", "E": "", "B": "b", "N": "3"}
	ok := []struct{ in, want string }{
		{"sin variables", "sin variables"},
		{"${A}", "a"},
		{"$A", "a"},
		{"x${A}y$B-z", "xay" + "b-z"},
		{"${U:-x}", "x"},
		{"${U-x}", "x"},
		{"${E:-x}", "x"},
		{"${E-x}", ""},
		{"${A:-x}", "a"},
		{"${A-x}", "a"},
		{"${U:+y}", ""},
		{"${E:+y}", ""},
		{"${A:+y}", "y"},
		{"${U+y}", ""},
		{"${E+y}", "y"},
		{"${E?}", ""},
		{"${A:?falta}", "a"},
		{"$$A", "$A"},
		{"$${A}", "${A}"},
		{"$1", "$1"},
		{"precio $", "precio $"},
		{"a$ b", "a$ b"},
		{"${A:-x}-${B}", "a-b"},
		{"${U:-x}-${B}", "x-b"},
		{"${A}${B}", "ab"},
		{`${U:-{"x":1}}`, `{"x":1}`},
		{"${U:-}", ""},
		{"ñandú ${A} ü", "ñandú a ü"},
	}
	for _, c := range ok {
		got, err := strictSubstitute(c.in, mapping(vars))
		if err != nil || got != c.want {
			t.Errorf("%q → %q, %v; esperaba %q", c.in, got, err, c.want)
		}
	}

	undefined := []struct{ in, name string }{
		{"${U}", "U"},
		{"$U", "U"},
		{"x-$U-y", "U"},
		{"${A:-x}-${U}", "U"},
		{"${A}${U}", "U"},
		{"${U:-x}-$V", "V"},
	}
	for _, c := range undefined {
		_, err := strictSubstitute(c.in, mapping(vars))
		ue, ok := errors.AsType[*UndefinedError](err)
		if !ok || ue.Name != c.name {
			t.Errorf("%q: esperaba *UndefinedError de %s, got %v", c.in, c.name, err)
		}
	}

	for _, in := range []string{"${U:?falta U}", "${E:?falta U}", "${U?falta U}"} {
		_, err := strictSubstitute(in, mapping(vars))
		re, ok := errors.AsType[*RequiredError](err)
		if !ok || re.Name != "U" && re.Name != "E" || !strings.Contains(err.Error(), "falta U") {
			t.Errorf("%q: esperaba *RequiredError con el motivo, got %v", in, err)
		}
	}

	// Formato inválido, y un $ dentro de un valor por defecto aunque la
	// variable exista (compose-go lo resolvería sin ser estricto).
	for _, in := range []string{"${}", "${A B}", "${1A}", "${A", "x ${A:-y", "${A:x}", "${A:-${B}}", "${A:-$B}", "${U:-$B}", "${U:+$$}", "${A:?falta $B}", "${{A}}"} {
		_, err := strictSubstitute(in, mapping(vars))
		if _, ok := errors.AsType[*TemplateError](err); !ok {
			t.Errorf("%q: esperaba *TemplateError, got %v", in, err)
		}
		if err := checkTemplate(in); err == nil {
			t.Errorf("checkTemplate(%q) debe fallar", in)
		}
	}
	// checkTemplate no busca valores.
	for _, in := range []string{"${U}", "$U", "${U:?x}", "$$", "${A:-x}-${U}"} {
		if err := checkTemplate(in); err != nil {
			t.Errorf("checkTemplate(%q): %v", in, err)
		}
	}
}

func TestStrictInterpolation(t *testing.T) {
	ctx := context.Background()
	var logs bytes.Buffer
	prev := logrus.StandardLogger().Out
	logrus.SetOutput(&logs)
	t.Cleanup(func() { logrus.SetOutput(prev) })

	load := func(src string, env map[string]string) (*File, error) {
		return Load(ctx, src, "p_s_a", env)
	}
	env := map[string]string{"A": "a", "E": "", "B": "b"}

	f, err := load(web("    environment:\n      X: ${A}\n      Y: $$A\n      Z: ${U:-def}\n      W: \"${E-x}\"\n      V: \"$$1 y $$\"\n"), env)
	if err != nil {
		t.Fatal(err)
	}
	got := f.Services["web"].Environment
	if got["X"] != "a" || got["Y"] != "$A" || got["Z"] != "def" || got["W"] != "" || got["V"] != "$1 y $" {
		t.Fatalf("environment = %v", got)
	}
	out, err := Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"Y": "$$A"`) {
		t.Fatalf("Marshal debe volver a escribir $$A:\n%s", out)
	}

	errs := []struct{ src, want string }{
		{web("    environment:\n      X: ${U}\n"), "compose_yaml: services.web.environment.X: la variable U no está definida: agrégala en las variables de la app o del stage, o usa ${U:-valor}"},
		{web("    environment:\n      X: $U\n"), "compose_yaml: services.web.environment.X: la variable U no está definida"},
		{web("    environment:\n      X: ${A:-x}-${U}\n"), "la variable U no está definida"},
		{web("    environment:\n      X: ${U:?pon U en el stage}\n"), "compose_yaml: services.web.environment.X: falta la variable U: pon U en el stage"},
		{web("    environment:\n      X: \"${A:-${B}}\"\n"), "compose_yaml: services.web.environment.X: un valor por defecto no puede tener $"},
		{web("    environment:\n      X: \"${}\"\n"), "compose_yaml: services.web.environment.X: "},
		{web("    labels:\n      com.example.v: ${U}\n"), "compose_yaml: services.web.labels.com.example.v: la variable U no está definida"},
		{web("    command: [\"a\", \"${U}\"]\n"), "compose_yaml: services.web.command: la variable U no está definida"},
		{web("    deploy:\n      replicas: ${U}\n"), "compose_yaml: services.web.deploy.replicas: la variable U no está definida"},
		// Con dos errores, uno solo: el de la primera ruta.
		{"services:\n  b:\n    image: nginx\n    environment: {X: \"${U2}\"}\n  a:\n    image: nginx\n    environment: {X: \"${U1}\"}\n",
			"compose_yaml: services.a.environment.X: la variable U1 no está definida"},
		// Los errores de conversión de compose-go van tal cual.
		{web("    deploy:\n      replicas: ${N}\n"), "compose_yaml: el compose no es válido: error while interpolating services.web.deploy.replicas: failed to cast"},
	}
	for _, c := range errs {
		_, err := load(c.src, map[string]string{"A": "a", "B": "b", "N": "abc"})
		ve := wantInvalid(t, err, c.want)
		if strings.Contains(ve.Error(), "👻") || strings.Contains(ve.Error(), "\n") {
			t.Errorf("mensaje con 👻 o saltos de línea: %q", ve.Error())
		}
		if strings.Count(ve.Error(), "no está definida") > 1 {
			t.Errorf("un solo error: %q", ve.Error())
		}
	}
	if strings.Contains(logs.String(), "variable is not set") {
		t.Fatalf("no debe escribir en logrus:\n%s", logs.String())
	}
}
