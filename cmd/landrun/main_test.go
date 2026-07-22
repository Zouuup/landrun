package main

import (
	"os"
	"reflect"
	"testing"
)

func TestProcessEnvironmentVars(t *testing.T) {
	t.Setenv("LANDRUN_TEST_ENV_A", "value-a")
	os.Unsetenv("LANDRUN_TEST_ENV_MISSING")

	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "empty",
			in:   nil,
			want: []string{},
		},
		{
			name: "key equals value",
			in:   []string{"FOO=bar"},
			want: []string{"FOO=bar"},
		},
		{
			name: "key from environment",
			in:   []string{"LANDRUN_TEST_ENV_A"},
			want: []string{"LANDRUN_TEST_ENV_A=value-a"},
		},
		{
			name: "missing key omitted",
			in:   []string{"LANDRUN_TEST_ENV_MISSING"},
			want: []string{},
		},
		{
			name: "mixed",
			in:   []string{"LANDRUN_TEST_ENV_A", "CUSTOM=1", "LANDRUN_TEST_ENV_MISSING"},
			want: []string{"LANDRUN_TEST_ENV_A=value-a", "CUSTOM=1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := processEnvironmentVars(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v want %#v", got, tc.want)
			}
		})
	}
}
