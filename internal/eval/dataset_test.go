package eval

import (
	"strings"
	"testing"
)

func TestLoadQueries_ParsesExtendedSchema(t *testing.T) {
	input := `{"id":"redis-01","query":"Redis 连接超时怎么排查","expected":[{"doc_id":"redis-timeout","relevance":3}],"answerable":true,"required_facts":["检查实例存活"],"language":"zh","category":"cache"}
{"id":"mysql-01","query":"MySQL 从库延迟","expected_doc_ids":["mysql-replication"]}`
	queries, err := LoadQueries(strings.NewReader(input))
	if err != nil {
		t.Fatalf("LoadQueries: %v", err)
	}
	if len(queries) != 2 {
		t.Fatalf("len = %d, want 2", len(queries))
	}
	if queries[0].ExpectedSet()["redis-timeout"] != 3 {
		t.Fatalf("relevance = %v, want 3", queries[0].ExpectedSet()["redis-timeout"])
	}
	// expected_doc_ids 并入 expected 且 relevance=1。
	if queries[1].ExpectedSet()["mysql-replication"] != 1 {
		t.Fatalf("expected_doc_ids relevance = %v, want 1", queries[1].ExpectedSet()["mysql-replication"])
	}
	if len(queries[1].Expected) != 1 {
		t.Fatalf("expected merge produced %d returns, want 1", len(queries[1].Expected))
	}
}

func TestLoadQueries_IgnoresBlankLines(t *testing.T) {
	input := "\n\n{\"id\":\"a\",\"query\":\"q\",\"expected_doc_ids\":[\"d\"]}\n"
	queries, err := LoadQueries(strings.NewReader(input))
	if err != nil {
		t.Fatalf("LoadQueries: %v", err)
	}
	if len(queries) != 1 {
		t.Fatalf("len = %d, want 1", len(queries))
	}
}

func TestLoadQueries_RejectsInvalidRows(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"missing id", `{"query":"q","expected_doc_ids":["d"]}`},
		{"missing query", `{"id":"a","expected_doc_ids":["d"]}`},
		{"missing expected", `{"id":"a","query":"q"}`},
		{"bad json", `{"id":`},
		{"empty dataset", ``},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := LoadQueries(strings.NewReader(tc.input)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestNormalize_DeduplicatesAndDefaultsRelevance(t *testing.T) {
	q := Query{
		ID:             "a",
		Query:          "q",
		Expected:       []ExpectedDoc{{DocID: "d"}},
		ExpectedDocIDs: []string{"d", "e", ""},
	}
	if err := q.Normalize(); err != nil {
		t.Fatal(err)
	}
	if len(q.Expected) != 2 {
		t.Fatalf("len = %d, want 2 (d,e)", len(q.Expected))
	}
	if q.ExpectedSet()["d"] != 1 {
		t.Fatalf("d relevance = %v, want 1", q.ExpectedSet()["d"])
	}
}

func TestNormalize_RejectsEmptyDocID(t *testing.T) {
	q := Query{ID: "a", Query: "q", Expected: []ExpectedDoc{{DocID: ""}}}
	if err := q.Normalize(); err == nil {
		t.Fatal("expected error for empty doc_id")
	}
}
