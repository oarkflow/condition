package routingfilebackedrunner

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/oarkflow/condition"
)

func Run(packagePath, requestPath string) {
	ctx := context.Background()
	pkg, err := condition.LoadDecisionPackageFile(ctx, condition.DecisionPackageFile{Path: packagePath})
	if err != nil {
		log.Fatal(err)
	}
	if diags := condition.ValidateDecisionPackage(pkg); condition.DiagnosticsHaveErrors(diags) {
		printJSON("validation_errors", diags)
		log.Fatal("package validation failed")
	}
	req, err := condition.LoadDecisionRequestFile(ctx, requestPath)
	if err != nil {
		log.Fatal(err)
	}

	orch := condition.NewDecisionOrchestrator()
	if err := orch.AddPackage(pkg); err != nil {
		log.Fatal(err)
	}
	res, err := orch.Evaluate(ctx, req)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("loaded package: %s@%s from %s\n", pkg.Name, pkg.Version, packagePath)
	fmt.Printf("loaded request: decision=%s from %s\n", req.Decision, requestPath)
	if res.Rank != nil {
		fmt.Printf("decision result: effect=%s winner=%s score=%.4f\n", res.Effect, res.Rank.ID, res.Rank.Score)
	} else {
		fmt.Printf("decision result: effect=%s score=%.4f\n", res.Effect, res.Score)
	}

	printRoutingViews(ctx, pkg, req)
	printJSON("decision_response", res)
}

func printRoutingViews(ctx context.Context, pkg condition.DecisionPackage, req condition.DecisionRequest) {
	ranking, ok := findRanking(pkg, req.Decision)
	if !ok {
		return
	}
	candidates := req.Candidates
	if len(candidates) == 0 {
		candidates = candidatesFromDataset(pkg, ranking.Dataset)
	}
	if len(candidates) == 0 {
		return
	}
	opts := []condition.RankerOption{
		condition.WithRankerTrace(true),
		condition.WithRoutingTieBreakerPaths(ranking.PriorityPath, ranking.SpecificityPath, ranking.FallbackPath, ranking.CostPath),
	}
	if ranking.WeightPath != "" && ranking.HashKeyPath != "" {
		opts = append(opts, condition.WithWeightedSelection(ranking.WeightPath, ranking.HashKeyPath))
	}
	ranker, err := condition.NewRanker(ranking.RuleSet, opts...)
	if err != nil {
		log.Fatal(err)
	}
	rankReq := condition.RankingRequest{Facts: req.Facts, Variables: req.Variables, Candidates: candidates}
	fallbacks, err := ranker.SelectFallbacks(ctx, rankReq, 3)
	if err != nil {
		log.Fatal(err)
	}
	printJSON("fallbacks", fallbacks)
	if ranking.WeightPath != "" {
		weighted, ok, err := ranker.SelectWeightedID(ctx, rankReq)
		if err != nil {
			log.Fatal(err)
		}
		if ok {
			printJSON("weighted_selection", weighted)
		}
	}
}

func findRanking(pkg condition.DecisionPackage, decision string) (condition.Ranking, bool) {
	for _, ranking := range pkg.Rankings {
		if ranking.Decision == decision || ranking.Name == decision || decision == "" {
			return ranking, true
		}
	}
	return condition.Ranking{}, false
}

func candidatesFromDataset(pkg condition.DecisionPackage, name string) []condition.Candidate {
	for _, dataset := range pkg.Datasets {
		if dataset.Name != name {
			continue
		}
		out := make([]condition.Candidate, 0, len(dataset.Records))
		for _, record := range dataset.Records {
			out = append(out, condition.Candidate{ID: record.ID, Name: record.Name, Facts: record.Facts, Metadata: record.Metadata})
		}
		return out
	}
	return nil
}

func printJSON(label string, v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("\n%s:\n%s\n", label, data)
}
