package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"

	"github.com/oarkflow/condition"
)

func main() {
	ctx := context.Background()

	engine := condition.NewEngine()
	rules := condition.StringSource(`{
		"name": "checkout",
		"rules": [
			{"id": "approve", "condition": "user.age >= 18 and order.total > 100", "decision": "approve"}
		]
	}`)
	if err := engine.AddRuleSetFrom(ctx, rules, condition.JSONDecoder[condition.RuleSet]()); err != nil {
		log.Fatal(err)
	}

	facts, err := condition.LoadFacts(ctx, condition.StringSource("user.age,order.total,tags.0\n29,149.9,vip\n"), condition.CSVFactsDecoder())
	if err != nil {
		log.Fatal(err)
	}
	result, err := engine.Evaluate(ctx, facts, "checkout")
	if err != nil {
		log.Fatal(err)
	}
	printJSON("csv facts result", result)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"name":"remote","rules":[{"id":"always","condition":"true","decision":"remote_ok"}]}`))
	}))
	defer server.Close()

	remote := condition.NewEngine()
	if err := remote.AddRuleSetFrom(ctx, condition.HTTPSource{URL: server.URL, Client: server.Client()}, condition.JSONDecoder[condition.RuleSet]()); err != nil {
		log.Fatal(err)
	}
	lazyFacts := condition.FactFunc(func(path string) (any, bool) {
		values := map[string]any{"anything": true}
		value, ok := values[path]
		return value, ok
	})
	remoteResult, err := remote.Evaluate(ctx, lazyFacts, "remote")
	if err != nil {
		log.Fatal(err)
	}
	printJSON("http rules result", remoteResult)
}

func printJSON(title string, v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s:\n%s\n", title, data)
}
