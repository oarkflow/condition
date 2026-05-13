package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/oarkflow/condition"
)

func main() {
	engine := condition.NewEngine(
		condition.WithExpressionOptions(condition.WithTrace(true)),
		condition.WithActionHandler("notify", func(_ context.Context, action condition.Action, _ condition.Facts) (condition.Event, error) {
			return condition.Event{
				Type:    "notification_queued",
				Payload: action.Payload,
			}, nil
		}),
	)

	rules := condition.RuleSet{
		Name: "checkout",
		Rules: []condition.Rule{
			{
				ID:       "approve-vip",
				Priority: 100,
				Group: &condition.Group{
					Mode: condition.GroupAll,
					Rules: []condition.Rule{
						{ID: "adult", Condition: `user.age >= 18`},
						{ID: "vip", Condition: `user.tier == "vip"`},
						{ID: "high-value", Condition: `order.total >= 100`},
					},
				},
				Score:       10,
				Decision:    "approve",
				Actions:     []condition.Action{{Type: "notify", Payload: map[string]any{"channel": "checkout", "template": "vip-approved"}}},
				StopOnMatch: true,
			},
			{
				ID:        "manual-review",
				Priority:  50,
				Condition: `order.total >= 500`,
				Score:     5,
				Decision:  "manual_review",
			},
		},
		Workflows: []condition.Workflow{
			{
				Name:       "post-checkout",
				StartStage: "start",
				Stages: []condition.Stage{
					{
						Name: "start",
						Rules: []condition.Rule{{
							ID:        "route-loyalty",
							Condition: `"vip" in tags`,
							NextStage: "loyalty",
						}},
					},
					{
						Name: "loyalty",
						Rules: []condition.Rule{{
							ID:        "award-points",
							Condition: `true`,
							Events:    []condition.Event{{Type: "points_awarded", Payload: map[string]any{"points": 150}}},
						}},
					},
				},
			},
		},
	}

	if err := engine.AddRuleSet(rules); err != nil {
		log.Fatal(err)
	}

	result, err := engine.Evaluate(context.Background(), condition.MapFacts{
		"user":  map[string]any{"age": 29, "tier": "vip"},
		"order": map[string]any{"total": 149.90},
		"tags":  []string{"vip", "returning"},
	}, "checkout")
	if err != nil {
		log.Fatal(err)
	}

	printJSON("ruleset result", result)
}

func printJSON(title string, v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s:\n%s\n", title, data)
}
