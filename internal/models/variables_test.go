package models

import (
	"reflect"
	"testing"
)

func TestParseVariables(t *testing.T) {
	tests := []struct {
		name    string
		keys    []string
		values  []string
		want    Variables
		wantErr bool
	}{
		{
			name:   "valid variables",
			keys:   []string{"REMOTE_HOST", "PROJECT_NAME"},
			values: []string{"172.16.10.200", "demo"},
			want:   Variables{"REMOTE_HOST": "172.16.10.200", "PROJECT_NAME": "demo"},
		},
		{
			name:   "ignore empty key",
			keys:   []string{"", "REMOTE_PORT"},
			values: []string{"ignored", "22"},
			want:   Variables{"REMOTE_PORT": "22"},
		},
		{
			name:    "invalid key",
			keys:    []string{"1_BAD"},
			values:  []string{"value"},
			wantErr: true,
		},
		{
			name:    "duplicate key",
			keys:    []string{"REMOTE_HOST", "REMOTE_HOST"},
			values:  []string{"one", "two"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseVariables(tt.keys, tt.values)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestMergeVariables(t *testing.T) {
	got := MergeVariables(
		Variables{"PROJECT_NAME": "builtin", "REMOTE_HOST": "default"},
		Variables{"REMOTE_HOST": "script", "REMOTE_PORT": "22"},
		Variables{"REMOTE_HOST": "ci"},
		Variables{"REMOTE_HOST": "runtime"},
	)
	want := Variables{"PROJECT_NAME": "builtin", "REMOTE_HOST": "runtime", "REMOTE_PORT": "22"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
