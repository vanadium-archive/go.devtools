package envutil

import (
	"os"
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

func TestCopyEmpty(t *testing.T) {
	tests := []map[string]string{
		nil,
		{},
	}
	for _, test := range tests {
		got := Copy(test)
		if got == nil {
			t.Errorf("Copy(%#v) got nil, which should never happen", test)
		}
		if got, want := len(got), 0; got != want {
			t.Errorf("Copy(%#v) got len %d, want %d", test, got, want)
		}
	}
}

func TestCopy(t *testing.T) {
	tests := []map[string]string{
		{},
		{"A": "1", "B": "2"},
		{"A": "1", "B": "2", "C": "3"},
	}
	for _, want := range tests {
		got := Copy(want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Copy got %v, want %v", got, want)
		}
		// Make sure we haven't just returned the original input map.
		got["test"] = "foo"
		if reflect.DeepEqual(got, want) {
			t.Errorf("Copy got %v, which is the input map", got)
		}
	}
}

type keyVal struct {
	Key, Val string
}

func testSnapshotGet(t *testing.T, s *Snapshot, tests []keyVal) {
	for _, kv := range tests {
		if got, want := s.Get(kv.Key), kv.Val; got != want {
			t.Errorf(`Get(%q) got %v, want %v`, kv.Key, got, want)
		}
	}
}

type keyTok struct {
	Key, Sep string
	Tok      []string
}

func testSnapshotGetTokens(t *testing.T, s *Snapshot, tests []keyTok) {
	for _, kt := range tests {
		if got, want := s.GetTokens(kt.Key, kt.Sep), kt.Tok; !reflect.DeepEqual(got, want) {
			t.Errorf(`GetTokens(%q, %q) got %v, want %v`, kt.Key, kt.Sep, got, want)
		}
	}
}

func TestSnapshotEmpty(t *testing.T) {
	s := NewSnapshot(nil)
	if got, want := s.Map(), map[string]string{}; !reflect.DeepEqual(got, want) {
		t.Errorf("Map got %v, want %v", got, want)
	}
	if got, want := s.Slice(), []string(nil); !reflect.DeepEqual(got, want) {
		t.Errorf("Slice got %v, want %v", got, want)
	}
	if got, want := s.BaseMap(), map[string]string{}; !reflect.DeepEqual(got, want) {
		t.Errorf("BaseMap got %v, want %v", got, want)
	}
	if got, want := s.DeltaMap(), map[string]string{}; !reflect.DeepEqual(got, want) {
		t.Errorf("DeltaMap got %v, want %v", got, want)
	}
	testSnapshotGet(t, s, []keyVal{{"noexist", ""}})
	testSnapshotGetTokens(t, s, []keyTok{{"noexist", ":", nil}})
}

func TestSnapshotNoSet(t *testing.T) {
	base := map[string]string{"A": "", "B": "foo", "C": "1:2:3"}
	s := NewSnapshot(base)
	if got, want := s.Map(), base; !reflect.DeepEqual(got, want) {
		t.Errorf("Map got %v, want %v", got, want)
	}
	if got, want := s.Slice(), []string{"A=", "B=foo", "C=1:2:3"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Slice got %v, want %v", got, want)
	}
	if got, want := s.BaseMap(), base; !reflect.DeepEqual(got, want) {
		t.Errorf("BaseMap got %v, want %v", got, want)
	}
	if got, want := s.DeltaMap(), map[string]string{}; !reflect.DeepEqual(got, want) {
		t.Errorf("DeltaMap got %v, want %v", got, want)
	}
	testSnapshotGet(t, s, []keyVal{
		{"A", ""},
		{"B", "foo"},
		{"C", "1:2:3"},
		{"noexist", ""},
	})
	testSnapshotGetTokens(t, s, []keyTok{
		{"A", ":", nil},
		{"A", " ", nil},
		{"B", ":", []string{"foo"}},
		{"B", " ", []string{"foo"}},
		{"C", ":", []string{"1", "2", "3"}},
		{"C", " ", []string{"1:2:3"}},
		{"noexist", ":", nil},
		{"noexist", " ", nil},
	})
}

func TestSnapshotWithSet(t *testing.T) {
	base := map[string]string{"A": "", "B": "foo", "C": "1:2:3"}
	s := NewSnapshot(base)
	s.SetTokens("B", []string{"a", "b", "c"}, ":")
	s.Set("C", "bar")
	s.Set("D", "baz")
	final := map[string]string{"A": "", "B": "a:b:c", "C": "bar", "D": "baz"}
	if got, want := s.Map(), final; !reflect.DeepEqual(got, want) {
		t.Errorf("Map got %v, want %v", got, want)
	}
	if got, want := s.Slice(), []string{"A=", "B=a:b:c", "C=bar", "D=baz"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Slice got %v, want %v", got, want)
	}
	if got, want := s.BaseMap(), base; !reflect.DeepEqual(got, want) {
		t.Errorf("BaseMap got %v, want %v", got, want)
	}
	delta := Copy(final)
	delete(delta, "A")
	if got, want := s.DeltaMap(), delta; !reflect.DeepEqual(got, want) {
		t.Errorf("DeltaMap got %v, want %v", got, want)
	}
	testSnapshotGet(t, s, []keyVal{
		{"A", ""},
		{"B", "a:b:c"},
		{"C", "bar"},
		{"D", "baz"},
		{"noexist", ""},
	})
	testSnapshotGetTokens(t, s, []keyTok{
		{"A", ":", nil},
		{"A", " ", nil},
		{"B", ":", []string{"a", "b", "c"}},
		{"B", " ", []string{"a:b:c"}},
		{"C", ":", []string{"bar"}},
		{"C", " ", []string{"bar"}},
		{"D", ":", []string{"baz"}},
		{"D", " ", []string{"baz"}},
		{"noexist", ":", nil},
		{"noexist", " ", nil},
	})
}

func TestNewSnapshotFromOS(t *testing.T) {
	// Just set an environment variable and make sure it shows up.
	const testKey, testVal = "OS_ENV_TEST_KEY", "OS_ENV_TEST_VAL"
	if err := os.Setenv(testKey, testVal); err != nil {
		t.Fatalf("Setenv(%q, %q) failed: %v", testKey, testVal, err)
	}
	s := NewSnapshotFromOS()
	if got, want := s.Get(testKey), testVal; got != want {
		t.Errorf("Get(%q) got %q, want %q", testKey, got, want)
	}
}
