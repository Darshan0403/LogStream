package api

import (
	"strings"
	"testing"

	"github.com/logstream/internal/models"
)

func TestValidateRule(t *testing.T) {
	ok := models.AlertRule{Name: "  disk  ", Pattern: "disk full", CooldownMinutes: 5}
	if msg := validateRule(&ok); msg != "" {
		t.Fatalf("valid rule rejected: %s", msg)
	}
	if ok.Name != "disk" {
		t.Fatalf("name not trimmed: %q", ok.Name)
	}

	bad := []models.AlertRule{
		{Name: "", Pattern: "x"},
		{Name: "n", Pattern: ""},
		{Name: "n", Pattern: "("},                             // invalid regex
		{Name: "n", Pattern: "x", CooldownMinutes: -1},        // negative cooldown
		{Name: "n", Pattern: "x", CooldownMinutes: 999999999}, // absurd cooldown
		{Name: strings.Repeat("a", 300), Pattern: "x"},        // name too long
		{Name: "n", Pattern: strings.Repeat("a", 600)},        // pattern too long
	}
	for i, r := range bad {
		if msg := validateRule(&r); msg == "" {
			t.Errorf("bad rule %d accepted", i)
		}
	}
}

func TestBearerFromProtocols(t *testing.T) {
	if got := bearerFromProtocols("logstream, auth.abc.def.ghi"); got != "abc.def.ghi" {
		t.Fatalf("got %q", got)
	}
	if got := bearerFromProtocols("  auth.tok123  "); got != "tok123" {
		t.Fatalf("got %q", got)
	}
	if got := bearerFromProtocols("logstream"); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if got := bearerFromProtocols(""); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}
