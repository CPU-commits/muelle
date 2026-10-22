package entity

import (
	"context"
	"testing"
)

func TestAllows(t *testing.T) {
	session := Principal{UserID: 1, Username: "felipe"}
	projectToken := Principal{Token: &TokenScope{ID: 7, Name: "ci", ProjectSlug: "auditfy"}}
	stageToken := Principal{Token: &TokenScope{ID: 8, Name: "ci-staging", ProjectSlug: "auditfy", StageSlug: "staging"}}
	// Un TokenScope sin ProjectSlug sería un bug de identity.
	noProjectToken := Principal{Token: &TokenScope{ID: 9, Name: "roto"}}
	noProjectStageToken := Principal{Token: &TokenScope{ID: 10, Name: "roto", StageSlug: "staging"}}

	cases := []struct {
		name           string
		p              Principal
		project, stage string
		write          bool
		want           bool
	}{
		{"sesión, proyecto", session, "auditfy", "", false, true},
		{"sesión, escribir en otro proyecto", session, "otro", "production", true, true},
		{"sesión, sin proyecto", session, "", "", true, true},
		{"token de proyecto, leer proyecto", projectToken, "auditfy", "", false, true},
		{"token de proyecto, escribir en stage", projectToken, "auditfy", "staging", true, true},
		{"token de proyecto, otro proyecto", projectToken, "otro", "", false, false},
		{"token de stage, escribir en su stage", stageToken, "auditfy", "staging", true, true},
		{"token de stage, leer otro stage", stageToken, "auditfy", "production", false, false},
		{"token de stage, leer proyecto", stageToken, "auditfy", "", false, true},
		{"token de stage, escribir en proyecto", stageToken, "auditfy", "", true, false},
		{"token de stage, otro proyecto mismo stage", stageToken, "otro", "staging", false, false},
		// Un slug vacío nunca autoriza a un token, aunque coincidan.
		{"token de proyecto, sin proyecto", projectToken, "", "", false, false},
		{"token de stage, sin proyecto", stageToken, "", "staging", false, false},
		{"token sin proyecto, sin proyecto", noProjectToken, "", "", false, false},
		{"token sin proyecto, leer proyecto", noProjectToken, "auditfy", "", false, false},
		{"token sin proyecto con stage, sin proyecto", noProjectStageToken, "", "staging", false, false},
	}
	for _, c := range cases {
		if got := c.p.Allows(c.project, c.stage, c.write); got != c.want {
			t.Errorf("%s: Allows(%q, %q, %v) = %v, esperaba %v", c.name, c.project, c.stage, c.write, got, c.want)
		}
	}
}

// La matriz de §5.5 del diseño del MCP (contrato §8.1): los cinco tokens
// sobre p1 (stages s1 y s2) y p2, más los dos AllProjects que no deberían
// existir (sin ReadOnly, o con slugs), que no autorizan nada.
func TestAllowsReadOnlyAndAllProjects(t *testing.T) {
	session := Principal{UserID: 1, Username: "felipe"}
	p := Principal{Token: &TokenScope{ID: 1, Name: "p", ProjectSlug: "p1"}}
	ps := Principal{Token: &TokenScope{ID: 2, Name: "ps", ProjectSlug: "p1", StageSlug: "s1"}}
	pl := Principal{Token: &TokenScope{ID: 3, Name: "pl", ProjectSlug: "p1", ReadOnly: true}}
	psl := Principal{Token: &TokenScope{ID: 4, Name: "psl", ProjectSlug: "p1", StageSlug: "s1", ReadOnly: true}}
	tl := Principal{Token: &TokenScope{ID: 5, Name: "tl", ReadOnly: true, AllProjects: true}}
	allWrite := Principal{Token: &TokenScope{ID: 6, Name: "roto", AllProjects: true}}
	allSlugs := Principal{Token: &TokenScope{ID: 7, Name: "roto", ProjectSlug: "p1", ReadOnly: true, AllProjects: true}}
	allStage := Principal{Token: &TokenScope{ID: 8, Name: "roto", StageSlug: "s1", ReadOnly: true, AllProjects: true}}

	type cell struct {
		project, stage string
		write          bool
	}
	cells := []cell{
		{"p1", "", false}, {"p1", "", true},
		{"p1", "s1", false}, {"p1", "s1", true},
		{"p1", "s2", false}, {"p1", "s2", true},
		{"p2", "", false}, {"p2", "s1", false}, {"p2", "s1", true},
		{"", "", false}, {"", "s1", false},
	}
	// want[i] es la respuesta para cells[i].
	cases := []struct {
		name string
		p    Principal
		want []bool
	}{
		{"sesión", session, []bool{true, true, true, true, true, true, true, true, true, true, true}},
		{"P", p, []bool{true, true, true, true, true, true, false, false, false, false, false}},
		{"PS", ps, []bool{true, false, true, true, false, false, false, false, false, false, false}},
		{"P·L", pl, []bool{true, false, true, false, true, false, false, false, false, false, false}},
		{"PS·L", psl, []bool{true, false, true, false, false, false, false, false, false, false, false}},
		{"T·L", tl, []bool{true, false, true, false, true, false, true, true, false, false, false}},
		{"todos sin solo lectura", allWrite, []bool{false, false, false, false, false, false, false, false, false, false, false}},
		{"todos con proyecto", allSlugs, []bool{false, false, false, false, false, false, false, false, false, false, false}},
		{"todos con stage", allStage, []bool{false, false, false, false, false, false, false, false, false, false, false}},
	}
	for _, c := range cases {
		for i, cl := range cells {
			if got := c.p.Allows(cl.project, cl.stage, cl.write); got != c.want[i] {
				t.Errorf("%s: Allows(%q, %q, %v) = %v, esperaba %v", c.name, cl.project, cl.stage, cl.write, got, c.want[i])
			}
		}
	}
}

func TestPrincipalReadOnly(t *testing.T) {
	cases := []struct {
		name string
		p    Principal
		want bool
	}{
		{"sesión", Principal{UserID: 1}, false},
		{"token de escritura", Principal{Token: &TokenScope{ID: 1, ProjectSlug: "p1"}}, false},
		{"token de solo lectura", Principal{Token: &TokenScope{ID: 1, ProjectSlug: "p1", ReadOnly: true}}, true},
		{"token de todos los proyectos", Principal{Token: &TokenScope{ID: 1, ReadOnly: true, AllProjects: true}}, true},
	}
	for _, c := range cases {
		if got := c.p.ReadOnly(); got != c.want {
			t.Errorf("%s: ReadOnly() = %v, esperaba %v", c.name, got, c.want)
		}
	}
}

func TestVia(t *testing.T) {
	if _, ok := ViaFrom(context.Background()); ok {
		t.Fatal("un ctx vacío no tiene via")
	}
	ctx := WithVia(context.Background(), ViaMCP)
	if via, ok := ViaFrom(ctx); !ok || via != "mcp" {
		t.Fatalf("ViaFrom = %q, %v", via, ok)
	}
	// El Principal y via van juntos en el mismo ctx.
	ctx = WithPrincipal(ctx, Principal{UserID: 2})
	if via, ok := ViaFrom(ctx); !ok || via != ViaMCP {
		t.Fatalf("ViaFrom con Principal = %q, %v", via, ok)
	}
}

func TestActor(t *testing.T) {
	typ, id := Principal{UserID: 3, Token: &TokenScope{ID: 9}}.Actor()
	if typ != ActorToken || id == nil || *id != 9 {
		t.Errorf("token: %v %v", typ, id)
	}
	typ, id = Principal{UserID: 3, Username: "felipe"}.Actor()
	if typ != ActorUser || id == nil || *id != 3 {
		t.Errorf("usuario: %v %v", typ, id)
	}
	typ, id = Principal{}.Actor()
	if typ != ActorSystem || id != nil {
		t.Errorf("sistema: %v %v", typ, id)
	}
}

// ActorName sigue a Actor: el nombre del token aunque haya usuario, el
// username de una sesión y "system" sin ninguno de los dos.
func TestActorName(t *testing.T) {
	cases := []struct {
		name string
		p    Principal
		want string
	}{
		{"token", Principal{UserID: 3, Username: "felipe", Token: &TokenScope{ID: 9, Name: "ci"}}, "ci"},
		{"usuario", Principal{UserID: 3, Username: "felipe"}, "felipe"},
		{"usuario con 2FA a medias", Principal{UserID: 3, Username: "felipe", MFAPending: true}, "felipe"},
		{"sistema", Principal{}, "system"},
		// Sin UserID es el sistema, aunque traiga un username.
		{"sistema con username", Principal{Username: "felipe"}, "system"},
	}
	for _, c := range cases {
		if got := c.p.ActorName(); got != c.want {
			t.Errorf("%s: ActorName() = %q, esperaba %q", c.name, got, c.want)
		}
	}
	if ActorSystemName != string(ActorSystem) {
		t.Errorf("ActorSystemName = %q", ActorSystemName)
	}
}

func TestIsToken(t *testing.T) {
	if (Principal{UserID: 1}).IsToken() || !(Principal{Token: &TokenScope{ID: 1}}).IsToken() {
		t.Error("IsToken")
	}
}

func TestPrincipalContext(t *testing.T) {
	if _, ok := PrincipalFrom(context.Background()); ok {
		t.Fatal("un ctx vacío no tiene Principal")
	}
	want := Principal{UserID: 5, Username: "ana", NeedsTOTPSetup: true}
	got, ok := PrincipalFrom(WithPrincipal(context.Background(), want))
	if !ok || got.UserID != want.UserID || got.Username != want.Username || !got.NeedsTOTPSetup {
		t.Fatalf("PrincipalFrom = %+v, %v", got, ok)
	}
}
