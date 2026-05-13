package condition

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	oauthz "github.com/oarkflow/authz"
)

func TestNewProductionDecisionServerRejectsUnsafeConfig(t *testing.T) {
	if _, err := NewProductionDecisionServer(ProductionDecisionServerConfig{}); err == nil {
		t.Fatal("expected missing store error")
	}
	if _, err := NewProductionDecisionServer(ProductionDecisionServerConfig{
		Store:      NewMemoryDecisionStore(),
		Authorizer: AuthorizerFunc(func(context.Context, Principal, Permission, string) error { return nil }),
		Config:     DefaultProductionDecisionServerConfig(),
	}); err == nil {
		t.Fatal("expected memory store rejection")
	}
	store, err := NewSQLiteDecisionStore(filepath.Join(t.TempDir(), "prod.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := NewProductionDecisionServer(ProductionDecisionServerConfig{Store: store, Config: DefaultProductionDecisionServerConfig()}); err == nil {
		t.Fatal("expected missing authorizer error")
	}
	if _, err := NewProductionDecisionServer(ProductionDecisionServerConfig{
		Store:      store,
		Authorizer: AuthorizerFunc(func(context.Context, Principal, Permission, string) error { return nil }),
		Config:     DecisionServerConfig{MaxBodyBytes: 1 << 20, RequestTimeout: time.Second},
	}); err == nil {
		t.Fatal("expected content type enforcement error")
	}
}

func TestNewProductionDecisionServerAcceptsSQLiteAndAuthorizer(t *testing.T) {
	store, err := NewSQLiteDecisionStore(filepath.Join(t.TempDir(), "prod.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	server, err := NewProductionDecisionServer(ProductionDecisionServerConfig{
		Store:      store,
		Authorizer: NewPolicyAuthorizer([]AuthorizationPolicy{{Permission: PermissionPackageList}}),
		Config:     DefaultProductionDecisionServerConfig(),
	})
	if err != nil {
		t.Fatalf("NewProductionDecisionServer returned error: %v", err)
	}
	if server == nil {
		t.Fatal("server is nil")
	}
}

func TestPolicyAuthorizer(t *testing.T) {
	authz := NewPolicyAuthorizer([]AuthorizationPolicy{{
		Role:        "operator",
		Permission:  PermissionDecisionEval,
		Resource:    "fraud/",
		Tenant:      "acme",
		AllowPrefix: true,
	}})
	p := Principal{ID: "operator", Tenant: "acme", Roles: []Role{"operator"}}
	if err := authz.Authorize(context.Background(), p, PermissionDecisionEval, "fraud/review"); err != nil {
		t.Fatalf("Authorize returned error: %v", err)
	}
	if err := authz.Authorize(context.Background(), p, PermissionPackagePublish, "packages"); err == nil {
		t.Fatal("expected publish denial")
	}
	if err := authz.Authorize(context.Background(), Principal{ID: "u2", Tenant: "other", Roles: []Role{"operator"}}, PermissionDecisionEval, "fraud/review"); err == nil {
		t.Fatal("expected tenant denial")
	}
}

type fakeAuthzEngine struct {
	allow bool
}

func (f fakeAuthzEngine) Authorize(context.Context, *oauthz.Subject, oauthz.Action, *oauthz.Resource, *oauthz.Environment) (*oauthz.Decision, error) {
	return &oauthz.Decision{Allowed: f.allow, Reason: "fake denial"}, nil
}

func TestAuthzAuthorizerAdapter(t *testing.T) {
	allow := NewAuthzAuthorizer(fakeAuthzEngine{allow: true})
	if err := allow.Authorize(context.Background(), Principal{ID: "u1", Tenant: "acme"}, PermissionPackageRead, "pkg"); err != nil {
		t.Fatalf("Authorize allow returned error: %v", err)
	}
	deny := NewAuthzAuthorizer(fakeAuthzEngine{allow: false})
	if err := deny.Authorize(context.Background(), Principal{ID: "u1", Tenant: "acme"}, PermissionPackageRead, "pkg"); err == nil {
		t.Fatal("expected deny error")
	}
}

func TestAuthzAuthorizerFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "condition.authz")
	data := []byte(`
tenant acme "ACME"

policy operator-evaluate acme allow decision:evaluate condition:fraud/* subject.roles@operator priority:50
policy operator-list acme allow package:list condition:packages subject.roles@operator priority:40
`)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	authorizer, err := NewAuthzAuthorizerFromFile(context.Background(), path)
	if err != nil {
		t.Fatalf("NewAuthzAuthorizerFromFile returned error: %v", err)
	}
	p := Principal{ID: "operator", Tenant: "acme", Roles: []Role{"operator"}}
	if err := authorizer.Authorize(context.Background(), p, PermissionDecisionEval, "fraud/review"); err != nil {
		t.Fatalf("file-backed authz evaluate returned error: %v", err)
	}
	if err := authorizer.Authorize(context.Background(), p, PermissionPackagePublish, "packages"); err == nil {
		t.Fatal("expected file-backed authz publish denial")
	}
}
