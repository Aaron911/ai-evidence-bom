package main

import "testing"

func TestIsLoopbackListen(t *testing.T) {
	tests := []struct {
		address string
		want    bool
	}{
		{"127.0.0.1:4318", true},
		{"[::1]:4318", true},
		{"localhost:4318", true},
		{":4318", false},
		{"0.0.0.0:4318", false},
		{"invalid", false},
	}
	for _, test := range tests {
		if got := isLoopbackListen(test.address); got != test.want {
			t.Errorf("isLoopbackListen(%q)=%v want=%v", test.address, got, test.want)
		}
	}
}
