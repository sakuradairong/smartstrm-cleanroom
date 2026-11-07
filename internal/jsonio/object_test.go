package jsonio

import (
	"strings"
	"testing"
)

func TestDecodeObjectBoundaries(t *testing.T) {
	for _, body := range []string{"", "null", "[]", `{"value":1} {}`, `{"value":1} garbage`, `{"unknown":1}`, `{"value":"wrong"}`, `{"value":1}` + strings.Repeat(" ", 64)} {
		t.Run(body, func(t *testing.T) {
			var target struct {
				Value int `json:"value"`
			}
			if err := DecodeObject(strings.NewReader(body), &target, 64); err == nil {
				t.Fatal("accepted invalid or oversized body")
			}
		})
	}
	var target struct {
		Value int `json:"value"`
	}
	body := `{"value":1}`
	if err := DecodeObject(strings.NewReader(body), &target, int64(len(body))); err != nil || target.Value != 1 {
		t.Fatalf("exact limit rejected: %+v %v", target, err)
	}
}
