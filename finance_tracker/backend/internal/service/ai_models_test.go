package service

import (
	"reflect"
	"testing"
)

func TestParseModelList(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []string
	}{
		{"openai data/id", `{"data":[{"id":"claude-opus-5"},{"id":"claude-sonnet-5"}]}`, []string{"claude-opus-5", "claude-sonnet-5"}},
		{"models strings", `{"models":["b-model","a-model"]}`, []string{"a-model", "b-model"}},
		{"bare array of objects", `[{"id":"m2"},{"name":"m1"}]`, []string{"m1", "m2"}},
		{"dedup + sort", `{"data":[{"id":"z"},{"id":"z"},{"id":"a"}]}`, []string{"a", "z"}},
	}
	for _, c := range cases {
		got, err := parseModelList([]byte(c.body))
		if err != nil {
			t.Errorf("%s: unexpected error %v", c.name, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
	if _, err := parseModelList([]byte(`{"data":[]}`)); err == nil {
		t.Error("empty list should error so the UI falls back to free text")
	}
	if _, err := parseModelList([]byte(`not json`)); err == nil {
		t.Error("garbage should error")
	}
}
