package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/oarkflow/condition"
	"github.com/oarkflow/condition/bcl"
)

func main() {
	ctx := context.Background()
	root := "examples/bcl/packages"
	entries, err := os.ReadDir(root)
	if err != nil {
		log.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".bcl") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		pkg, err := bcl.LoadPackageFile(path)
		if err != nil {
			log.Fatalf("%s: parse: %v", path, err)
		}
		diags := condition.ValidateDecisionPackage(pkg)
		if condition.DiagnosticsHaveErrors(diags) {
			log.Fatalf("%s: diagnostics: %#v", path, diags)
		}
		result, err := condition.RunDecisionPackageTests(ctx, pkg)
		if err != nil {
			log.Fatalf("%s: tests: %v", path, err)
		}
		digest, err := condition.PackageDigest(pkg)
		if err != nil {
			log.Fatalf("%s: digest: %v", path, err)
		}
		fmt.Printf("%-28s package=%-24s digest=%s tests=%d passed=%v\n", entry.Name(), pkg.Name, digest[:12], result.Total, result.Passed)
	}
}
