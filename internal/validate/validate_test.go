package validate

import (
	"errors"
	"strings"
	"testing"
)

func TestSlugs(t *testing.T) {
	ok := []string{"a", "auditfy", "aumi-agents", "a1", "abcdefghijklmnop"}
	bad := []string{"", "1abc", "-abc", "abc-", "Auditfy", "audit_fy", "abcdefghijklmnopq", "a.b"}
	for _, s := range ok {
		if err := ProjectSlug(s); err != nil {
			t.Errorf("ProjectSlug(%q) = %v", s, err)
		}
		if err := AppSlug(s); err != nil {
			t.Errorf("AppSlug(%q) = %v", s, err)
		}
	}
	for _, s := range bad {
		if ProjectSlug(s) == nil {
			t.Errorf("ProjectSlug(%q) debió fallar", s)
		}
	}
	if StageSlug("production") != nil || StageSlug("abcdefghijkl") != nil {
		t.Error("stage válidos rechazados")
	}
	if StageSlug("abcdefghijklm") == nil {
		t.Error("stage de 13 caracteres debió fallar")
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

func TestUsernamePasswordEnvKeyName(t *testing.T) {
	if Username("felipe") != nil || Username("ab") == nil || Username("Felipe") == nil {
		t.Error("Username")
	}
	if Password("corta") == nil || Password("una-contraseña-larga") != nil || Password(strings.Repeat("x", 257)) == nil {
		t.Error("Password")
	}
	if EnvKey("DATABASE_URL") != nil || EnvKey("_X") != nil || EnvKey("1X") == nil || EnvKey("A-B") == nil {
		t.Error("EnvKey")
	}
	if Name("name", "Auditfy", 64) != nil || Name("name", "  ", 64) == nil || Name("name", strings.Repeat("x", 65), 64) == nil {
		t.Error("Name")
	}
}
