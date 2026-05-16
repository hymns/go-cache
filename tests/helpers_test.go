package cache_test

import (
	"reflect"
	"testing"
)

func noErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func isTrue(t *testing.T, v bool) {
	t.Helper()
	if !v {
		t.Fatal("want true, got false")
	}
}

func isFalse(t *testing.T, v bool) {
	t.Helper()
	if v {
		t.Fatal("want false, got true")
	}
}

func eq(t *testing.T, want, got any) {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("want %v, got %v", want, got)
	}
}
