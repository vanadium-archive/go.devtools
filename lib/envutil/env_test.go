package envutil

import (
	"reflect"
	"testing"
)

func TestToMap(t *testing.T) {
	tests := []struct {
		Slice []string
		Map   map[string]string
	}{
		{nil, nil},
		{[]string{``}, nil},
		{
			[]string{
				``,
				`A`,
				`B=`,
				`C=3`,
				`D==`,
				`E==5`,
				`F=6=`,
				`G=7=7`,
				`H="8"`,
			},
			map[string]string{
				`A`: ``,
				`B`: ``,
				`C`: `3`,
				`D`: `=`,
				`E`: `=5`,
				`F`: `6=`,
				`G`: `7=7`,
				`H`: `"8"`,
			},
		},
	}
	for _, test := range tests {
		if got, want := ToMap(test.Slice), test.Map; !reflect.DeepEqual(got, want) {
			t.Errorf("ToMap got %v, want %v", got, want)
		}
	}
}

func TestToSlice(t *testing.T) {
	tests := []struct {
		Map   map[string]string
		Slice []string
	}{
		{nil, nil},
		{map[string]string{``: ``}, nil},
		{map[string]string{``: `foo`}, nil},
		{map[string]string{``: `foo`}, nil},
		{
			map[string]string{
				``:  ``,
				`A`: ``,
				`B`: ``,
				`C`: `3`,
				`D`: `=`,
				`E`: `=5`,
				`F`: `6=`,
				`G`: `7=7`,
				`H`: `"8"`,
			},
			[]string{
				`A=`,
				`B=`,
				`C=3`,
				`D==`,
				`E==5`,
				`F=6=`,
				`G=7=7`,
				`H="8"`,
			},
		},
	}
	for _, test := range tests {
		if got, want := ToSlice(test.Map), test.Slice; !reflect.DeepEqual(got, want) {
			t.Errorf("ToSlice got %v, want %v", got, want)
		}
	}
}

func TestToQuotedSlice(t *testing.T) {
	tests := []struct {
		Map   map[string]string
		Slice []string
	}{
		{nil, nil},
		{map[string]string{``: ``}, nil},
		{map[string]string{``: `foo`}, nil},
		{map[string]string{``: `foo`}, nil},
		{
			map[string]string{
				``:  ``,
				`A`: ``,
				`B`: ``,
				`C`: `3`,
				`D`: `=`,
				`E`: `=5`,
				`F`: `6=`,
				`G`: `7=7`,
				`H`: `"8"`,
			},
			[]string{
				`A=""`,
				`B=""`,
				`C="3"`,
				`D="="`,
				`E="=5"`,
				`F="6="`,
				`G="7=7"`,
				`H="\"8\""`,
			},
		},
	}
	for _, test := range tests {
		if got, want := ToQuotedSlice(test.Map), test.Slice; !reflect.DeepEqual(got, want) {
			t.Errorf("ToQuotedSlice got %v, want %v", got, want)
		}
	}
}
