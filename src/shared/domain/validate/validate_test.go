package validate

import (
	"errors"
	"strings"
	"testing"
)

var badSlugs = []string{"", "1abc", "-abc", "abc-", "Auditfy", "audit_fy", "abcdefghijklmnopq", "a.b"}

func TestSlugs(t *testing.T) {
	ok := []string{"a", "auditfy", "aumi-agents", "a1", "abcdefghijklmnop"}
	for _, s := range ok {
		if err := ProjectSlug(s); err != nil {
			t.Errorf("ProjectSlug(%q) = %v", s, err)
		}
		if err := AppSlug(s); err != nil {
			t.Errorf("AppSlug(%q) = %v", s, err)
		}
	}
	for _, s := range badSlugs {
		if ProjectSlug(s) == nil {
			t.Errorf("ProjectSlug(%q) debió fallar", s)
		}
		if AppSlug(s) == nil {
			t.Errorf("AppSlug(%q) debió fallar", s)
		}
		if StageSlug(s) == nil {
			t.Errorf("StageSlug(%q) debió fallar", s)
		}
	}
	if StageSlug("production") != nil || StageSlug("abcdefghijkl") != nil {
		t.Error("stage válidos rechazados")
	}
	if StageSlug("abcdefghijklm") == nil {
		t.Error("stage de 13 caracteres debió fallar")
	}
}

func TestReservedProjectSlugs(t *testing.T) {
	for _, s := range []string{"muelle", "docker"} {
		err := ProjectSlug(s)
		var ve *Error
		if !errors.As(err, &ve) || ve.Field != "slug" || !strings.Contains(ve.Message, "está reservado") {
			t.Errorf("ProjectSlug(%q) = %v, esperaba que estuviera reservado", s, err)
		}
		// Solo los proyectos tienen nombres reservados.
		if err := AppSlug(s); err != nil {
			t.Errorf("AppSlug(%q) = %v", s, err)
		}
		if err := StageSlug(s); err != nil {
			t.Errorf("StageSlug(%q) = %v", s, err)
		}
	}
}

func TestVolumeName(t *testing.T) {
	for _, s := range []string{"data", "pg-data", "a", "abcdefghijklmnop"} {
		if err := VolumeName("volumes[0].name", s); err != nil {
			t.Errorf("VolumeName(%q) = %v", s, err)
		}
	}
	for _, s := range badSlugs {
		err := VolumeName("volumes[0].name", s)
		var ve *Error
		if !errors.As(err, &ve) || ve.Field != "volumes[0].name" {
			t.Errorf("VolumeName(%q) = %v, esperaba *Error con el campo indicado", s, err)
		}
	}
}

// El servicio Swarm más largo posible (proyecto_stage_app_servicio) cabe en 63 caracteres.
func TestLongestSwarmServiceName(t *testing.T) {
	project := "p" + strings.Repeat("a", 15)
	stage := "s" + strings.Repeat("a", 11)
	app := "a" + strings.Repeat("a", 15)
	service := "v" + strings.Repeat("a", 15)
	if ProjectSlug(project) != nil || StageSlug(stage) != nil || AppSlug(app) != nil || VolumeName("services", service) != nil {
		t.Fatal("los slugs de largo máximo deben ser válidos")
	}
	for _, longer := range []error{
		ProjectSlug(project + "a"), StageSlug(stage + "a"), AppSlug(app + "a"), VolumeName("services", service+"a"),
	} {
		if longer == nil {
			t.Fatal("un slug más largo que el máximo debió fallar")
		}
	}
	name := project + "_" + stage + "_" + app + "_" + service
	if len(name) > 63 {
		t.Fatalf("%q tiene %d caracteres, el máximo de Swarm es 63", name, len(name))
	}
}

func TestErrorType(t *testing.T) {
	err := ProjectSlug("X")
	var ve *Error
	if !errors.As(err, &ve) || ve.Field != "slug" {
		t.Fatalf("esperaba *Error con Field=slug, got %#v", err)
	}
	if !strings.HasPrefix(err.Error(), "slug: ") {
		t.Fatalf("mensaje: %q", err.Error())
	}
}

func TestUsername(t *testing.T) {
	if Username("felipe") != nil || Username("ab") == nil || Username("Felipe") == nil {
		t.Error("Username")
	}
	if err := Username("a" + strings.Repeat("b", 31)); err != nil {
		t.Errorf("Username de 32 caracteres debió pasar: %v", err)
	}
	if Username("a"+strings.Repeat("b", 32)) == nil {
		t.Error("Username de 33 caracteres debió fallar")
	}
}

func TestPassword(t *testing.T) {
	if Password("corta") == nil || Password("una-contraseña-larga") != nil {
		t.Error("Password")
	}
	// El mínimo cuenta runas, no bytes.
	if Password(strings.Repeat("ñ", 11)) == nil {
		t.Error("11 runas (22 bytes) debió fallar")
	}
	if err := Password(strings.Repeat("ñ", 12)); err != nil {
		t.Errorf("12 runas debió pasar: %v", err)
	}
	// El máximo cuenta bytes.
	if err := Password(strings.Repeat("x", 256)); err != nil {
		t.Errorf("256 bytes debió pasar: %v", err)
	}
	if Password(strings.Repeat("x", 257)) == nil {
		t.Error("257 bytes debió fallar")
	}
}

func TestEnvKey(t *testing.T) {
	for _, k := range []string{"DATABASE_URL", "_X", "a", "COMPOSE", "MY_COMPOSE_FILE", "A" + strings.Repeat("B", 127)} {
		if err := EnvKey("vars", k); err != nil {
			t.Errorf("EnvKey(%q) = %v", k, err)
		}
	}
	for _, k := range []string{"", "1X", "A-B", "A B", "Ñ", "A" + strings.Repeat("B", 128)} {
		if EnvKey("vars", k) == nil {
			t.Errorf("EnvKey(%q) debió fallar", k)
		}
	}
}

func TestEnvKeyReservesComposePrefix(t *testing.T) {
	for _, k := range []string{"COMPOSE_PROJECT_NAME", "COMPOSE_PROFILES", "COMPOSE_"} {
		err := EnvKey("vars", k)
		var ve *Error
		if !errors.As(err, &ve) || !strings.Contains(ve.Message, "COMPOSE_") {
			t.Errorf("EnvKey(%q) = %v, esperaba el prefijo reservado", k, err)
		}
	}
}

func TestEnvKeyUsesField(t *testing.T) {
	for _, field := range []string{"vars", "set", "unset"} {
		err := EnvKey(field, "1X")
		var ve *Error
		if !errors.As(err, &ve) || ve.Field != field {
			t.Errorf("EnvKey(%q, \"1X\") = %v, esperaba Field=%s", field, err, field)
		}
	}
}

func TestName(t *testing.T) {
	if Name("name", "Auditfy", 64) != nil || Name("name", "  ", 64) == nil || Name("name", strings.Repeat("x", 65), 64) == nil {
		t.Error("Name")
	}
	if err := Name("name", "Proyecto ñandú", 64); err != nil {
		t.Errorf("Name con acentos debió pasar: %v", err)
	}
	invisible := []string{
		"\x00", "a\nb", "a\tb",
		"\u200b",    // espacio de ancho cero (Cf)
		"\u202eabc", // invierte el texto (Cf)
		"a\u2028b",  // separador de línea (Zl)
		"a\u2029b",  // separador de párrafo (Zp)
		"\u3164",    // relleno Hangul
		"a\u115fb",  // relleno inicial Hangul
		"a\u1160b",  // relleno medial Hangul
		"\uffa0",    // relleno Hangul de ancho medio
	}
	for _, s := range invisible {
		err := Name("name", s, 64)
		var ve *Error
		if !errors.As(err, &ve) || ve.Field != "name" {
			t.Errorf("Name(%q) = %v, esperaba que rechazara el carácter invisible", s, err)
		}
	}
	for _, s := range []string{"a\xffb", "\xc3", "ok\xed\xa0\x80"} {
		err := Name("name", s, 64)
		var ve *Error
		if !errors.As(err, &ve) || ve.Field != "name" || !strings.Contains(ve.Message, "UTF-8") {
			t.Errorf("Name(%q) = %v, esperaba que rechazara el UTF-8 inválido", s, err)
		}
	}
}
