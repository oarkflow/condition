package condition

import (
	"context"
	"fmt"
	"sync"
)

type StoreConformanceT interface {
	Helper()
	Fatalf(string, ...any)
}

func StoreConformanceSuite(t StoreConformanceT, store DecisionStore) {
	t.Helper()
	ctx := context.Background()
	pkg := DecisionPackage{Name: "store-conformance", Version: "1", Environment: "test"}
	digest, err := PackageDigest(pkg)
	if err != nil {
		t.Fatalf("PackageDigest returned error: %v", err)
	}
	if err := store.SavePackage(ctx, pkg, digest); err != nil {
		t.Fatalf("SavePackage returned error: %v", err)
	}
	got, ok, err := store.GetPackage(ctx, pkg.Name, pkg.Version, pkg.Environment)
	if err != nil || !ok || got.Digest != digest {
		t.Fatalf("GetPackage returned record=%#v ok=%v err=%v", got, ok, err)
	}
	latest, ok, err := store.GetPackage(ctx, pkg.Name, "", pkg.Environment)
	if err != nil || !ok || latest.Package.Version != "1" {
		t.Fatalf("latest GetPackage returned record=%#v ok=%v err=%v", latest, ok, err)
	}
	audit := AuditRecord{Package: pkg.Name, Version: pkg.Version, PackageDigest: digest}
	env1, err := store.SaveAuditEnvelope(ctx, audit)
	if err != nil {
		t.Fatalf("SaveAuditEnvelope returned error: %v", err)
	}
	env2, err := store.SaveAuditEnvelope(ctx, audit)
	if err != nil {
		t.Fatalf("second SaveAuditEnvelope returned error: %v", err)
	}
	if env2.Sequence != env1.Sequence+1 || env2.PreviousHash != env1.ChainHash {
		t.Fatalf("audit chain not contiguous: %#v %#v", env1, env2)
	}
	envelopes, err := store.ListAuditEnvelopes(ctx)
	if err != nil {
		t.Fatalf("ListAuditEnvelopes returned error: %v", err)
	}
	if err := VerifyAuditChain(envelopes); err != nil {
		t.Fatalf("VerifyAuditChain returned error: %v", err)
	}
	if _, err := store.SaveSimulation(ctx, SimulationResult{}); err != nil {
		t.Fatalf("SaveSimulation returned error: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			next := DecisionPackage{Name: "store-conformance", Version: fmt.Sprintf("c%d", i), Environment: "test"}
			nextDigest, _ := PackageDigest(next)
			_ = store.SavePackage(ctx, next, nextDigest)
			_, _ = store.SaveAuditEnvelope(ctx, audit)
		}(i)
	}
	wg.Wait()
	if _, err := store.ListPackages(ctx); err != nil {
		t.Fatalf("ListPackages after concurrent access returned error: %v", err)
	}
}
