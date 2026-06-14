package main

import "testing"

func TestSpecialSymbolsRegistered(t *testing.T) {
	if symbolByName["W"] != symWild {
		t.Errorf("W should map to symWild")
	}
	if symbolByName["S"] != symScatter {
		t.Errorf("S should map to symScatter")
	}
	if faceArt[symWild] == "" || faceArt[symScatter] == "" {
		t.Errorf("wild/scatter need reel art")
	}
}
