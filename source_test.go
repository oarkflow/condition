package condition

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestLoadRuleSetJSON(t *testing.T) {
	original := RuleSet{
		Name: "json",
		Rules: []Rule{{
			ID:        "adult",
			Condition: `user.age >= 18`,
			Decision:  "allow",
		}},
	}
	data, err := original.JSON()
	if err != nil {
		t.Fatalf("JSON returned error: %v", err)
	}
	loaded, err := LoadRuleSet(context.Background(), BytesSource(data), JSONDecoder[RuleSet]())
	if err != nil {
		t.Fatalf("LoadRuleSet returned error: %v", err)
	}
	if !reflect.DeepEqual(original, loaded) {
		t.Fatalf("loaded ruleset = %#v, want %#v", loaded, original)
	}
}

func TestLoadCSVFactsDotPaths(t *testing.T) {
	facts, err := LoadFacts(context.Background(), StringSource("user.age,order.total,tags.0\n29,149.9,vip\n"), CSVFactsDecoder())
	if err != nil {
		t.Fatalf("LoadFacts returned error: %v", err)
	}
	matched, err := MustCompile(`user.age >= 18 and order.total > 100 and "vip" in tags`).EvalBool(context.Background(), facts)
	if err != nil {
		t.Fatalf("EvalBool returned error: %v", err)
	}
	if !matched {
		t.Fatal("expected CSV facts to match")
	}
}

func TestHTTPSource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Context().Err() != nil {
			t.Fatal(r.Context().Err())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"http","rules":[{"id":"r1","condition":"true"}]}`))
	}))
	defer server.Close()

	loaded, err := LoadRuleSet(context.Background(), HTTPSource{URL: server.URL, Client: server.Client()}, JSONDecoder[RuleSet]())
	if err != nil {
		t.Fatalf("LoadRuleSet returned error: %v", err)
	}
	if loaded.Name != "http" || len(loaded.Rules) != 1 {
		t.Fatalf("unexpected ruleset: %#v", loaded)
	}
}

func TestEngineReloadRuleSetFromKeepsOldRuleSetOnError(t *testing.T) {
	engine := NewEngine()
	if err := engine.AddRuleSetFrom(context.Background(), StringSource(`{"name":"checkout","rules":[{"id":"old","condition":"user.age >= 18","decision":"allow"}]}`), JSONDecoder[RuleSet]()); err != nil {
		t.Fatalf("AddRuleSetFrom returned error: %v", err)
	}
	if err := engine.ReloadRuleSetFrom(context.Background(), StringSource(`{"name":"checkout","rules":[{"condition":"true"}]}`), JSONDecoder[RuleSet]()); err == nil {
		t.Fatal("ReloadRuleSetFrom returned nil error for invalid rule")
	}
	result, err := engine.Evaluate(context.Background(), MapFacts{"user": map[string]any{"age": 18}}, "checkout")
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if !result.Matched || result.Decision != "allow" {
		t.Fatalf("old ruleset was not preserved: %#v", result)
	}
}

func TestLoadRowFacts(t *testing.T) {
	rows := fakeRows{
		columns: []string{"user.age", "order.total"},
		rows: []map[string]any{{
			"user.age":    29,
			"order.total": 149.9,
		}},
	}
	facts, err := LoadRowFacts(context.Background(), fakeRowSource{rows: &rows})
	if err != nil {
		t.Fatalf("LoadRowFacts returned error: %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("rows = %d, want 1", len(facts))
	}
	matched, err := MustCompile(`user.age >= 18 and order.total > 100`).EvalBool(context.Background(), facts[0])
	if err != nil {
		t.Fatalf("EvalBool returned error: %v", err)
	}
	if !matched {
		t.Fatal("expected row facts to match")
	}
}

func TestDynamicFacts(t *testing.T) {
	var gotErr error
	facts := DynamicFacts{
		Context: context.Background(),
		Source:  fakeFactSource{values: map[string]any{"user.age": 29}},
		OnError: func(err error) {
			gotErr = err
		},
	}
	matched, err := MustCompile(`user.age >= 18`).EvalBool(context.Background(), facts)
	if err != nil {
		t.Fatalf("EvalBool returned error: %v", err)
	}
	if !matched {
		t.Fatal("expected dynamic facts to match")
	}
	if gotErr != nil {
		t.Fatalf("OnError was called: %v", gotErr)
	}
}

type fakeRowSource struct {
	rows Rows
}

func (s fakeRowSource) Rows(context.Context) (Rows, error) { return s.rows, nil }

type fakeRows struct {
	columns []string
	rows    []map[string]any
	index   int
}

func (r *fakeRows) Columns() []string { return r.columns }

func (r *fakeRows) Next() (map[string]any, bool, error) {
	if r.index >= len(r.rows) {
		return nil, false, nil
	}
	row := r.rows[r.index]
	r.index++
	return row, true, nil
}

func (r *fakeRows) Close() error { return nil }

type fakeFactSource struct {
	values map[string]any
	err    error
}

func (s fakeFactSource) GetFact(context.Context, string) (any, bool, error) {
	if s.err != nil {
		return nil, false, s.err
	}
	value, ok := s.values["user.age"]
	return value, ok, nil
}

func TestDynamicFactsError(t *testing.T) {
	want := errors.New("boom")
	var got error
	facts := DynamicFacts{
		Source: fakeFactSource{err: want},
		OnError: func(err error) {
			got = err
		},
	}
	if _, ok := facts.Get("user.age"); ok {
		t.Fatal("expected missing fact on source error")
	}
	if !errors.Is(got, want) {
		t.Fatalf("OnError = %v, want %v", got, want)
	}
}
