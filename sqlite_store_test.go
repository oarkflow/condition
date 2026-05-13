package condition

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSQLiteDecisionStoreConformance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.db")
	store, err := NewSQLiteDecisionStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteDecisionStore returned error: %v", err)
	}
	defer store.Close()
	StoreConformanceSuite(t, store)
}

func TestSQLiteDecisionStorePersistsAcrossReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "decisions.db")
	store, err := NewSQLiteDecisionStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteDecisionStore returned error: %v", err)
	}
	pkg := DecisionPackage{Name: "persistent", Version: "1", Environment: "prod"}
	digest, err := PackageDigest(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SavePackage(ctx, pkg, digest); err != nil {
		t.Fatalf("SavePackage returned error: %v", err)
	}
	if _, err := store.SaveAuditEnvelope(ctx, AuditRecord{Package: pkg.Name, Version: pkg.Version, PackageDigest: digest}); err != nil {
		t.Fatalf("SaveAuditEnvelope returned error: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	reopened, err := NewSQLiteDecisionStore(path)
	if err != nil {
		t.Fatalf("reopen returned error: %v", err)
	}
	defer reopened.Close()
	record, ok, err := reopened.GetPackage(ctx, "persistent", "", "prod")
	if err != nil || !ok || record.Digest != digest {
		t.Fatalf("GetPackage after reopen record=%#v ok=%v err=%v", record, ok, err)
	}
	envelopes, err := reopened.ListAuditEnvelopes(ctx)
	if err != nil {
		t.Fatalf("ListAuditEnvelopes returned error: %v", err)
	}
	if err := VerifyAuditChain(envelopes); err != nil {
		t.Fatalf("VerifyAuditChain returned error: %v", err)
	}
}
