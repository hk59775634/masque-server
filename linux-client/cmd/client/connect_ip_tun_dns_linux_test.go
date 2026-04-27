//go:build linux

package main

import (
	"reflect"
	"testing"
)

func TestParseCommaList(t *testing.T) {
	got := parseCommaList(" 1.1.1.1 , 8.8.8.8 ")
	want := []string{"1.1.1.1", "8.8.8.8"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
	if parseCommaList("") != nil {
		t.Fatal("empty should be nil")
	}
}
