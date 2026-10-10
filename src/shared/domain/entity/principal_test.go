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
