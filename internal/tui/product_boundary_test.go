package tui

import (
	"reflect"
	"testing"
)

func TestBasicProductBoundary(t *testing.T) {
	if !reflect.DeepEqual(pageKeys, []string{"overview", "top", "traffic", "sources", "help"}) {
		t.Fatalf("advanced pages remain: %v", pageKeys)
	}
	if !reflect.DeepEqual(dims, []string{"host", "app", "proto"}) {
		t.Fatalf("advanced dimensions remain: %v", dims)
	}
	a := newApp(Options{Lang: "en"})
	if ranges[a.rng] != "1h" {
		t.Fatalf("default range: %s", ranges[a.rng])
	}
}
