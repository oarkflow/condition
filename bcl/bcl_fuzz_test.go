package bcl

import (
	"testing"

	"github.com/oarkflow/condition"
)

func FuzzParsePackage(f *testing.F) {
	f.Add(testBCLPackage)
	f.Add(`module "small" { rule_set "x" { rule "1" { when { true } then { decision = "allow" } } } }`)
	f.Add(`module "bad" { policy`)
	f.Fuzz(func(t *testing.T, src string) {
		_, _ = ParsePackage([]byte(src))
	})
}

func FuzzBCLRoundTrip(f *testing.F) {
	f.Add(testBCLPackage)
	f.Fuzz(func(t *testing.T, src string) {
		pkg, err := ParsePackage([]byte(src))
		if err != nil {
			return
		}
		encoded, err := EncodePackage(pkg)
		if err != nil {
			t.Fatalf("EncodePackage returned error: %v", err)
		}
		again, err := ParsePackage(encoded)
		if err != nil {
			t.Fatalf("ParsePackage(encoded) returned error: %v", err)
		}
		a, _ := condition.PackageDigest(pkg)
		b, _ := condition.PackageDigest(again)
		if a != b {
			t.Fatalf("digest mismatch after roundtrip: %s != %s", a, b)
		}
	})
}
