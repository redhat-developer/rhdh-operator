package main

import "testing"

func TestEolRelease(t *testing.T) {
	tags := []string{"1.6.0", "1.7.0", "1.8.0", "1.9.0", "1.10.0"}

	tests := []struct {
		tag     string
		want    string
		wantOK  bool
		wantErr bool
	}{
		{tag: "1.8.0", want: "release-1.6", wantOK: true},
		{tag: "1.10.0", want: "release-1.8", wantOK: true},
		{tag: "2.1.0", want: "release-1.9", wantOK: true},
		{tag: "1.1.0", wantOK: false},
		{tag: "1.8.1", wantErr: true},
	}

	for _, tt := range tests {
		got, ok, err := eolRelease(tt.tag, tags)
		if tt.wantErr {
			if err == nil {
				t.Fatalf("%s: expected error", tt.tag)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", tt.tag, err)
		}
		if ok != tt.wantOK || got != tt.want {
			t.Fatalf("%s: got (%q, %v), want (%q, %v)", tt.tag, got, ok, tt.want, tt.wantOK)
		}
	}
}
