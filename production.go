package condition

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type ProductionPackageValidationResult struct {
	Diagnostics []Diagnostic       `json:"diagnostics,omitempty"`
	Digest      string             `json:"digest,omitempty"`
	Tests       *PackageTestResult `json:"tests,omitempty"`
}

func ValidateProductionDecisionPackage(ctx context.Context, pkg DecisionPackage, opts ...PackageTestOption) (ProductionPackageValidationResult, error) {
	result := ProductionPackageValidationResult{Diagnostics: ValidateDecisionPackage(pkg)}
	if DiagnosticsHaveErrors(result.Diagnostics) {
		return result, fmt.Errorf("decision package validation failed")
	}
	digest, err := PackageDigest(pkg)
	if err != nil {
		return result, err
	}
	result.Digest = digest
	if _, err := CompileDecisionPackage(pkg); err != nil {
		return result, err
	}
	if len(pkg.Tests) > 0 {
		tests, err := RunDecisionPackageTests(ctx, pkg, opts...)
		result.Tests = &tests
		if err != nil {
			return result, err
		}
		if !tests.Passed {
			return result, fmt.Errorf("decision package tests failed: %d/%d failed", tests.Failed, tests.Total)
		}
	}
	return result, nil
}

type ProductionDecisionServerConfig struct {
	Store       DecisionStore
	Authorizer  Authorizer
	Config      DecisionServerConfig
	Metrics     DecisionMetrics
	Logger      DecisionLogger
	RateLimiter DecisionRateLimiter
}

func NewProductionDecisionServer(cfg ProductionDecisionServerConfig) (*DecisionServer, error) {
	if cfg.Store == nil {
		return nil, errors.New("condition: production decision server requires a durable DecisionStore")
	}
	if _, ok := cfg.Store.(*MemoryDecisionStore); ok {
		return nil, errors.New("condition: MemoryDecisionStore is development-only and cannot be used for production server")
	}
	if cfg.Authorizer == nil {
		return nil, errors.New("condition: production decision server requires an Authorizer")
	}
	if cfg.Config.MaxBodyBytes <= 0 {
		return nil, errors.New("condition: production decision server requires MaxBodyBytes")
	}
	if cfg.Config.RequestTimeout <= 0 {
		return nil, errors.New("condition: production decision server requires RequestTimeout")
	}
	if !cfg.Config.RequireContentType {
		return nil, errors.New("condition: production decision server requires content-type enforcement")
	}
	opts := []DecisionServerOption{
		WithDecisionServerStore(cfg.Store),
		WithDecisionServerAuthorizer(cfg.Authorizer),
		WithDecisionServerConfig(cfg.Config),
	}
	if cfg.Metrics != nil {
		opts = append(opts, WithDecisionServerMetrics(cfg.Metrics))
	}
	if cfg.Logger != nil {
		opts = append(opts, WithDecisionServerLogger(cfg.Logger))
	}
	if cfg.RateLimiter != nil {
		opts = append(opts, WithDecisionServerRateLimiter(cfg.RateLimiter))
	}
	return NewDecisionServer(opts...), nil
}

func DefaultProductionDecisionServerConfig() DecisionServerConfig {
	return DecisionServerConfig{
		MaxBodyBytes:       1 << 20,
		RequestTimeout:     10 * time.Second,
		RequireContentType: true,
		AllowedContentTypes: []string{
			"application/json",
		},
	}
}
