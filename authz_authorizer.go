package condition

import (
	"context"
	"errors"
	"fmt"
	"strings"

	oauthz "github.com/oarkflow/authz"
	"github.com/oarkflow/authz/stores"
)

type AuthzEngine interface {
	Authorize(context.Context, *oauthz.Subject, oauthz.Action, *oauthz.Resource, *oauthz.Environment) (*oauthz.Decision, error)
}

type AuthzAuthorizer struct {
	Engine AuthzEngine
}

func NewAuthzAuthorizer(engine AuthzEngine) Authorizer {
	return AuthzAuthorizer{Engine: engine}
}

func NewAuthzEngineFromFile(ctx context.Context, filename string) (*oauthz.Engine, error) {
	cfg, err := oauthz.NewDSLParser().ParseFile(filename)
	if err != nil {
		return nil, err
	}
	if err := oauthz.ValidateConfig(cfg); err != nil {
		return nil, err
	}
	engine := oauthz.NewEngine(
		stores.NewMemoryPolicyStore(),
		stores.NewMemoryRoleStore(),
		stores.NewMemoryACLStore(),
		stores.NewMemoryAuditStore(),
		oauthz.WithRoleMembershipStore(stores.NewMemoryRoleMembershipStore()),
		oauthz.WithTenantStore(stores.NewMemoryTenantStore()),
	)
	if err := engine.ApplyConfig(ctx, cfg); err != nil {
		return nil, err
	}
	return engine, nil
}

func NewAuthzAuthorizerFromFile(ctx context.Context, filename string) (Authorizer, error) {
	engine, err := NewAuthzEngineFromFile(ctx, filename)
	if err != nil {
		return nil, err
	}
	return NewAuthzAuthorizer(engine), nil
}

func (a AuthzAuthorizer) Authorize(ctx context.Context, p Principal, perm Permission, resource string) error {
	if a.Engine == nil {
		return errors.New("authz engine is required")
	}
	decision, err := a.Engine.Authorize(ctx, authzSubject(p), oauthz.Action(perm), authzResource(p, resource), &oauthz.Environment{TenantID: p.Tenant, Extra: map[string]any{"permission": string(perm)}})
	if err != nil {
		return err
	}
	if decision == nil || !decision.Allowed {
		if decision != nil && decision.Reason != "" {
			return errors.New(decision.Reason)
		}
		return fmt.Errorf("permission %s denied for %s", perm, resource)
	}
	return nil
}

func authzSubject(p Principal) *oauthz.Subject {
	roles := make([]string, 0, len(p.Roles))
	for _, role := range p.Roles {
		roles = append(roles, string(role))
	}
	attrs := cloneMap(p.Metadata)
	if len(roles) > 0 {
		if attrs == nil {
			attrs = map[string]any{}
		}
		attrs["role"] = roles[0]
	}
	return &oauthz.Subject{ID: p.ID, Type: "user", TenantID: p.Tenant, Roles: roles, Attrs: attrs}
}

func authzResource(p Principal, resource string) *oauthz.Resource {
	return &oauthz.Resource{ID: resource, Type: "condition", TenantID: p.Tenant}
}

type AuthorizationPolicy struct {
	Role        Role
	Permission  Permission
	Resource    string
	Tenant      string
	AllowPrefix bool
}

type PolicyAuthorizer struct {
	Policies []AuthorizationPolicy
}

func NewPolicyAuthorizer(policies []AuthorizationPolicy) Authorizer {
	return PolicyAuthorizer{Policies: policies}
}

func (a PolicyAuthorizer) Authorize(_ context.Context, p Principal, perm Permission, resource string) error {
	for _, policy := range a.Policies {
		if policy.Permission != perm {
			continue
		}
		if policy.Tenant != "" && policy.Tenant != p.Tenant {
			continue
		}
		if policy.Resource != "" {
			if policy.AllowPrefix {
				if !strings.HasPrefix(resource, policy.Resource) {
					continue
				}
			} else if policy.Resource != resource {
				continue
			}
		}
		if policy.Role == "" {
			return nil
		}
		for _, role := range p.Roles {
			if role == policy.Role {
				return nil
			}
		}
	}
	return fmt.Errorf("permission %s denied for %s", perm, resource)
}
