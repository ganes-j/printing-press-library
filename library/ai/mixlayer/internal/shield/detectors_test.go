// Copyright 2026 Cathryn Lavery and contributors. Licensed under Apache-2.0. See LICENSE.

package shield

import (
	"strings"
	"testing"
)

func TestDetectFindsDeterministicPII(t *testing.T) {
	text := "Cathryn Lavery used cathryn@example.com, 4111 1111 1111 1111, 192.168.1.1, and 123-45-6789."
	entities := Detect(text)
	kinds := map[string]bool{}
	for _, e := range entities {
		kinds[e.Kind] = true
	}
	for _, want := range []string{"PERSON", "EMAIL", "CARD", "IP", "SSN"} {
		if !kinds[want] {
			t.Fatalf("missing %s in %#v", want, entities)
		}
	}
	if got := RiskScore(entities); got != 10 {
		t.Fatalf("RiskScore() = %d, want 10", got)
	}
}

func TestRestructureCoarsensWithoutBucketingDates(t *testing.T) {
	input := "name,email,amount,date\nCathryn Lavery,cathryn@example.com,1234,2026-06-24\n"
	got, err := Restructure(input, RestructureOptions{BucketNumerics: true, CoarsenDates: "quarter", DropColumns: []string{"email"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "example.com") {
		t.Fatalf("email column not dropped: %s", got)
	}
	if !strings.Contains(got, "1000-9999") || !strings.Contains(got, "2026-Q2") {
		t.Fatalf("unexpected restructure output: %s", got)
	}
}
