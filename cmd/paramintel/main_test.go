package main

import (
	"reflect"
	"testing"
	"time"
)

func TestParseLocations(t *testing.T) {
	got, err := parseLocations("json, query,json")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"json", "query"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("locations=%v want=%v", got, want)
	}
	if _, err := parseLocations("headers"); err == nil {
		t.Fatal("expected invalid location error")
	}
}

func TestMethodMayChangeState(t *testing.T) {
	for _, method := range []string{"GET", "HEAD", "OPTIONS"} {
		if methodMayChangeState(method) {
			t.Fatalf("%s should be treated as non-state-changing", method)
		}
	}
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		if !methodMayChangeState(method) {
			t.Fatalf("%s should require explicit opt-in", method)
		}
	}
}

func TestValidateDelay(t *testing.T) {
	for _, delay := range []time.Duration{0, time.Millisecond, time.Second} {
		if err := validateDelay(delay); err != nil {
			t.Fatalf("delay=%v error=%v", delay, err)
		}
	}
	if err := validateDelay(-time.Millisecond); err == nil {
		t.Fatal("expected negative delay validation error")
	}
}
