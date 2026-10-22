package entity

import (
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

var t0 = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

func TestNewUser(t *testing.T) {
	creator := int64(3)
	u, err := NewUser("felipe", "hash", &creator, t0)
	if err != nil || u.Username != "felipe" || u.PasswordHash != "hash" || *u.CreatedBy != 3 || !u.CreatedAt.Equal(t0) {
		t.Fatalf("u=%+v err=%v", u, err)
	}
	if u.HasTOTP() || u.HasPendingTOTP() {
		t.Fatalf("un usuario nuevo no tiene 2FA: %+v", u)
	}
	for _, bad := range []string{"", "ab", "Felipe", "con espacio", strings.Repeat("a", 33)} {
		_, err := NewUser(bad, "hash", nil, t0)
		if ve, ok := errors.AsType[*validate.Error](err); !ok || ve.Field != "username" {
			t.Errorf("%q: esperaba un error de validación de username, got %v", bad, err)
		}
	}
}

func TestUserTOTP(t *testing.T) {
	u := User{TOTPSecret: []byte("x")}
	if !u.HasTOTP() || u.HasPendingTOTP() {
		t.Fatalf("u=%+v", u)
	}
	u = User{TOTPPending: []byte("x")}
	if u.HasTOTP() || !u.HasPendingTOTP() {
		t.Fatalf("u=%+v", u)
	}
}

func TestNewSession(t *testing.T) {
	s := NewSession("h", 7, false, t0, 7*24*time.Hour, "1.2.3.4", "curl")
	if s.IDHash != "h" || s.UserID != 7 || s.MFAPending || !s.CreatedAt.Equal(t0) || !s.LastSeenAt.Equal(t0) ||
		!s.ExpiresAt.Equal(t0.Add(7*24*time.Hour)) || s.IP != "1.2.3.4" || s.UserAgent != "curl" {
		t.Fatalf("s=%+v", s)
	}

	long := NewSession("h", 7, false, t0, time.Hour, "", strings.Repeat("a", 300))
	if len(long.UserAgent) != MaxUserAgent {
		t.Fatalf("user agent de %d bytes", len(long.UserAgent))
	}
	// No corta una runa por la mitad: 255 'a' + 'ñ' (2 bytes) queda en 255 bytes.
	cut := NewSession("h", 7, false, t0, time.Hour, "", strings.Repeat("a", 255)+"ñandú")
	if !utf8.ValidString(cut.UserAgent) || len(cut.UserAgent) != 255 {
		t.Fatalf("user agent %q (%d bytes)", cut.UserAgent, len(cut.UserAgent))
	}

	// El user agent y la IP vienen de headers: se guardan siempre como UTF-8
	// válido (json/v2 responde 500 si una respuesta tiene UTF-8 inválido).
	dirty := NewSession("h", 7, false, t0, time.Hour, "1.2.3.4\xff"+strings.Repeat("9", 100), "curl\xff\xfe/8")
	if dirty.UserAgent != "curl�/8" {
		t.Fatalf("user agent %q", dirty.UserAgent)
	}
	if !utf8.ValidString(dirty.IP) || len(dirty.IP) > MaxIP || !strings.HasPrefix(dirty.IP, "1.2.3.4�") {
		t.Fatalf("ip %q (%d bytes)", dirty.IP, len(dirty.IP))
	}
}

func TestSessionExpired(t *testing.T) {
	idle := 12 * time.Hour
	full := NewSession("h", 1, false, t0, 7*24*time.Hour, "", "")
	cases := []struct {
		name string
		s    Session
		at   time.Time
		want bool
	}{
		{"recién creada", full, t0, false},
		{"12 h sin uso justo", full, t0.Add(idle), false},
		{"12 h y 1 s sin uso", full, t0.Add(idle + time.Second), true},
		{"en el vencimiento", Session{LastSeenAt: t0.Add(7 * 24 * time.Hour), ExpiresAt: t0.Add(7 * 24 * time.Hour)}, t0.Add(7 * 24 * time.Hour), true},
		// Una sesión pendiente de MFA solo vence por su expires_at (5 min).
		{"pendiente sin uso", NewSession("p", 1, true, t0, 5*time.Minute, "", ""), t0.Add(4 * time.Minute), false},
		{"pendiente vencida", NewSession("p", 1, true, t0, 5*time.Minute, "", ""), t0.Add(5 * time.Minute), true},
	}
	for _, c := range cases {
		if got := c.s.Expired(c.at, idle); got != c.want {
			t.Errorf("%s: Expired=%v, esperaba %v", c.name, got, c.want)
		}
	}
}

func TestInvite(t *testing.T) {
	creator := int64(1)
	inv := NewInvite("h", &creator, t0, 24*time.Hour)
	if inv.TokenHash != "h" || *inv.CreatedBy != 1 || !inv.ExpiresAt.Equal(t0.Add(24*time.Hour)) || inv.UsedAt != nil || inv.IsSystem() {
		t.Fatalf("inv=%+v", inv)
	}
	if !NewInvite("h", nil, t0, time.Hour).IsSystem() {
		t.Fatal("sin creador es una invitación de setup del sistema")
	}
	if !inv.Usable(t0) || !inv.Usable(t0.Add(24*time.Hour-time.Millisecond)) {
		t.Fatal("antes de vencer debe servir")
	}
	if inv.Usable(t0.Add(24 * time.Hour)) {
		t.Fatal("vencida no sirve")
	}
	used := inv
	used.UsedAt = &t0
	if used.Usable(t0) {
		t.Fatal("usada no sirve")
	}
}

func TestNewAPIToken(t *testing.T) {
	stage, creator := int64(4), int64(1)
	exp := t0.Add(24 * time.Hour)
	tok, err := NewAPIToken("ci", "h", 2, &stage, &creator, t0, &exp, true, false)
	if err != nil || tok.Name != "ci" || tok.TokenHash != "h" || tok.ProjectID != 2 || *tok.StageID != 4 ||
		*tok.CreatedBy != 1 || !tok.CreatedAt.Equal(t0) || !tok.ExpiresAt.Equal(exp) || tok.LastUsedAt != nil ||
		!tok.ReadOnly || tok.AllProjects {
		t.Fatalf("tok=%+v err=%v", tok, err)
	}
	all, err := NewAPIToken("claude", "h", 0, nil, &creator, t0, nil, true, true)
	if err != nil || !all.ReadOnly || !all.AllProjects || all.ProjectID != 0 || all.StageID != nil {
		t.Fatalf("all=%+v err=%v", all, err)
	}
	for _, bad := range []string{"", "   ", strings.Repeat("a", MaxTokenName+1), "a\nb", "​", "ci\xff"} {
		_, err := NewAPIToken(bad, "h", 2, nil, nil, t0, nil, false, false)
		if ve, ok := errors.AsType[*validate.Error](err); !ok || ve.Field != "name" {
			t.Errorf("%q: esperaba un error de validación de name, got %v", bad, err)
		}
	}
}

// ScopeValid: un token siempre es de un proyecto, y tiene slug de stage si y
// solo si tiene stage. Si no, Allows lo trataría como de todo el proyecto.
func TestAPITokenScopeValid(t *testing.T) {
	stage := int64(4)
	cases := []struct {
		name string
		tok  APIToken
		want bool
	}{
		{"proyecto", APIToken{ProjectSlug: "auditfy"}, true},
		{"stage", APIToken{ProjectSlug: "auditfy", StageID: &stage, StageSlug: "staging"}, true},
		{"sin proyecto", APIToken{}, false},
		{"stage sin slug", APIToken{ProjectSlug: "auditfy", StageID: &stage}, false},
		{"slug sin stage", APIToken{ProjectSlug: "auditfy", StageSlug: "staging"}, false},
		{"proyecto de solo lectura", APIToken{ProjectSlug: "auditfy", ReadOnly: true}, true},
		{"stage de solo lectura", APIToken{ProjectSlug: "auditfy", StageID: &stage, StageSlug: "staging", ReadOnly: true}, true},
		{"todos los proyectos", APIToken{ReadOnly: true, AllProjects: true}, true},
		// Todos los proyectos va sin proyecto ni stage, y de solo lectura.
		{"todos sin solo lectura", APIToken{AllProjects: true}, false},
		{"todos con id de proyecto", APIToken{ProjectID: 2, ReadOnly: true, AllProjects: true}, false},
		{"todos con slug de proyecto", APIToken{ProjectSlug: "auditfy", ReadOnly: true, AllProjects: true}, false},
		{"todos con stage", APIToken{StageID: &stage, ReadOnly: true, AllProjects: true}, false},
		{"todos con slug de stage", APIToken{StageSlug: "staging", ReadOnly: true, AllProjects: true}, false},
	}
	for _, c := range cases {
		if got := c.tok.ScopeValid(); got != c.want {
			t.Errorf("%s: ScopeValid=%v, esperaba %v", c.name, got, c.want)
		}
	}
}

func TestAPITokenExpiredAndScope(t *testing.T) {
	exp := t0.Add(time.Hour)
	tok := APIToken{ID: 9, Name: "ci", ProjectSlug: "auditfy", StageSlug: "staging", ExpiresAt: &exp}
	if tok.Expired(t0) || !tok.Expired(exp) || !tok.Expired(exp.Add(time.Second)) {
		t.Fatal("vence en expires_at")
	}
	if (APIToken{}).Expired(t0.Add(100 * 365 * 24 * time.Hour)) {
		t.Fatal("sin expires_at no vence")
	}
	s := tok.Scope()
	if s.ID != 9 || s.Name != "ci" || s.ProjectSlug != "auditfy" || s.StageSlug != "staging" || s.ReadOnly || s.AllProjects {
		t.Fatalf("scope=%+v", s)
	}
	all := APIToken{ID: 10, Name: "claude", ReadOnly: true, AllProjects: true}.Scope()
	if all.ID != 10 || all.Name != "claude" || all.ProjectSlug != "" || all.StageSlug != "" || !all.ReadOnly || !all.AllProjects {
		t.Fatalf("scope de todos los proyectos=%+v", all)
	}
}
