package eval

import (
	"testing"
)

func TestBuildReport_GroupsAndCountsFailures(t *testing.T) {
	queries := []Query{
		{ID: "q1", Query: "a", Category: "cache", Language: "zh", Expected: []ExpectedDoc{{DocID: "d1"}}},
		{ID: "q2", Query: "b", Category: "cache", Language: "zh", Expected: []ExpectedDoc{{DocID: "d2"}}},
		{ID: "q3", Query: "c", Category: "db", Language: "en", Expected: []ExpectedDoc{{DocID: "d3"}}},
	}
	results := []QueryResult{
		{ID: "q1", Expected: map[string]float64{"d1": 1}, Retrieved: []string{"d1"}},
		{ID: "q2", Expected: map[string]float64{"d2": 1}, Retrieved: []string{"d2"}},
		{ID: "q3", Expected: map[string]float64{"d3": 1}, Retrieved: []string{"other"}},
	}
	cases := []CaseResult{
		{ID: "q1", Query: "a", Hit: true, Retrieved: []RetrievedDoc{{Rank: 0, DocID: "d1"}}},
		{ID: "q2", Query: "b", Hit: true, Retrieved: []RetrievedDoc{{Rank: 0, DocID: "d2"}}},
		{ID: "q3", Query: "c", Hit: false, Retrieved: []RetrievedDoc{{Rank: 0, DocID: "other"}}},
	}
	report, err := BuildReport(Evaluation{Queries: queries, Results: results, Cases: cases}, 5, ReportMeta{K: 5})
	if err != nil {
		t.Fatal(err)
	}
	if report.Total != 3 || report.HitCount != 2 || len(report.Failures) != 1 {
		t.Fatalf("total=%d hit=%d failures=%d, want 3/2/1", report.Total, report.HitCount, len(report.Failures))
	}
	if report.Failures[0].ID != "q3" {
		t.Fatalf("failure id = %q, want q3", report.Failures[0].ID)
	}
	if report.ByCategory["cache"].N != 2 || report.ByLanguage["en"].N != 1 {
		t.Fatalf("group summaries missing: %+v", report.ByCategory)
	}
	// cache 组 2 条全命中，hitrate 应为 1；(queries[:2] 都命中)
	if report.ByCategory["cache"].HitRateAtK != 1 {
		t.Fatalf("cache HitRateAtK = %v, want 1", report.ByCategory["cache"].HitRateAtK)
	}
	if report.ByLanguage["en"].HitRateAtK != 0 {
		t.Fatalf("en HitRateAtK = %v, want 0", report.ByLanguage["en"].HitRateAtK)
	}
}

func TestBuildReport_RejectsOutOfSyncSlices(t *testing.T) {
	if _, err := BuildReport(Evaluation{Queries: []Query{{}}, Results: nil, Cases: nil}, 5, ReportMeta{}); err == nil {
		t.Fatal("expected error for out-of-sync slices")
	}
}

func TestBuildReport_CountsErrors(t *testing.T) {
	queries := []Query{{ID: "q1", Query: "a", Expected: []ExpectedDoc{{DocID: "d1"}}}}
	results := []QueryResult{{ID: "q1", Expected: map[string]float64{"d1": 1}, Retrieved: nil}}
	cases := []CaseResult{{ID: "q1", Query: "a", Hit: false}}
	report, err := BuildReport(Evaluation{Queries: queries, Results: results, Cases: cases}, 5, ReportMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Errors != 0 {
		t.Fatalf("Errors = %d, want 0", report.Errors)
	}
}
