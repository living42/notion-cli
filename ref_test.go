package main

import "testing"

// validUUID is a canonical dashed UUID used across the parseResourceRef tests.
const validUUID = "12345678-1234-1234-1234-123456789012"

// validHex32 is the compact 32-char hex form of validUUID.
const validHex32 = "12345678123412341234123456789012"

func TestParseResourceRef_PagePrefix(t *testing.T) {
	kind, id, err := parseResourceRef("page:" + validUUID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if kind != "page" {
		t.Errorf("kind = %q, want page", kind)
	}
	if id != validUUID {
		t.Errorf("id = %q, want %s", id, validUUID)
	}
}

func TestParseResourceRef_DatabasePrefixes(t *testing.T) {
	for _, prefix := range []string{"db", "database", "DB", "Database"} {
		kind, id, err := parseResourceRef(prefix + ":" + validHex32)
		if err != nil {
			t.Fatalf("prefix %q: unexpected error: %v", prefix, err)
		}
		if kind != "database" {
			t.Errorf("prefix %q: kind = %q, want database", prefix, kind)
		}
		// hex32 normalizes to dashed UUID
		if id != validUUID {
			t.Errorf("prefix %q: id = %q, want %s", prefix, id, validUUID)
		}
	}
}

func TestParseResourceRef_DataSourcePrefixes(t *testing.T) {
	for _, prefix := range []string{"ds", "data-source", "data_source"} {
		kind, id, err := parseResourceRef(prefix + ":" + validUUID)
		if err != nil {
			t.Fatalf("prefix %q: unexpected error: %v", prefix, err)
		}
		if kind != "data_source" {
			t.Errorf("prefix %q: kind = %q, want data_source", prefix, kind)
		}
		if id != validUUID {
			t.Errorf("prefix %q: id = %q, want %s", prefix, id, validUUID)
		}
	}
}

func TestParseResourceRef_BareIDDefaultsToPage(t *testing.T) {
	kind, id, err := parseResourceRef(validUUID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if kind != "page" {
		t.Errorf("kind = %q, want page (default)", kind)
	}
	if id != validUUID {
		t.Errorf("id = %q, want %s", id, validUUID)
	}
}

func TestParseResourceRef_BareHex32Normalizes(t *testing.T) {
	kind, id, err := parseResourceRef(validHex32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if kind != "page" {
		t.Errorf("kind = %q, want page", kind)
	}
	if id != validUUID {
		t.Errorf("id = %q, want %s", id, validUUID)
	}
}

func TestParseResourceRef_UppercaseIDLowercased(t *testing.T) {
	kind, id, err := parseResourceRef("PAGE:ABCDEF12-1234-1234-1234-123456789012")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if kind != "page" {
		t.Errorf("kind = %q, want page", kind)
	}
	want := "abcdef12-1234-1234-1234-123456789012"
	if id != want {
		t.Errorf("id = %q, want %s", id, want)
	}
}

func TestParseResourceRef_UnknownPrefix(t *testing.T) {
	if _, _, err := parseResourceRef("foo:" + validUUID); err == nil {
		t.Fatal("expected error for unknown prefix, got nil")
	}
}

func TestParseResourceRef_Empty(t *testing.T) {
	if _, _, err := parseResourceRef(""); err == nil {
		t.Fatal("expected error for empty ref, got nil")
	}
	if _, _, err := parseResourceRef("   "); err == nil {
		t.Fatal("expected error for whitespace ref, got nil")
	}
}

func TestParseResourceRef_InvalidID(t *testing.T) {
	if _, _, err := parseResourceRef("page:not-a-uuid"); err == nil {
		t.Fatal("expected error for invalid id with prefix, got nil")
	}
	if _, _, err := parseResourceRef("not-a-uuid"); err == nil {
		t.Fatal("expected error for invalid bare id, got nil")
	}
}

func TestParseResourceRef_EmptyIDAfterPrefix(t *testing.T) {
	if _, _, err := parseResourceRef("page:"); err == nil {
		t.Fatal("expected error for empty id after prefix, got nil")
	}
}

func TestKindLabel(t *testing.T) {
	cases := map[string]string{
		"page":        "page",
		"database":    "database",
		"data_source": "data source",
		"unknown":     "page",
	}
	for in, want := range cases {
		if got := kindLabel(in); got != want {
			t.Errorf("kindLabel(%q) = %q, want %q", in, got, want)
		}
	}
}
