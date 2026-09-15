package httpapi

import (
	"bytes"
	"encoding/base64"
	"net/url"
	"testing"
)

func TestAuditCursorCanonicalRoundTrip(t *testing.T) {
	cursor := encodeAuditCursor(42)
	sequenceID, err := decodeAuditCursor(cursor)
	if err != nil {
		t.Fatalf("decode canonical cursor: %v", err)
	}
	if sequenceID != 42 {
		t.Fatalf("sequence id=%d want=42", sequenceID)
	}
}

func TestAuditCursorRejectsAlternateSequenceText(t *testing.T) {
	for _, cursor := range []string{
		base64.RawURLEncoding.EncodeToString([]byte("v1:042")),
		base64.RawURLEncoding.EncodeToString([]byte("v1:+42")),
	} {
		if _, err := decodeAuditCursor(cursor); err == nil {
			t.Fatalf("non-canonical cursor %q was accepted", cursor)
		}
	}
}

func TestAuditCursorRejectsNonZeroBase64PaddingBits(t *testing.T) {
	canonical := encodeAuditCursor(42)
	decoded, err := base64.RawURLEncoding.DecodeString(canonical)
	if err != nil {
		t.Fatal(err)
	}

	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	variant := ""
	for _, replacement := range alphabet {
		candidate := canonical[:len(canonical)-1] + string(replacement)
		if candidate == canonical {
			continue
		}
		candidateDecoded, decodeErr := base64.RawURLEncoding.DecodeString(candidate)
		if decodeErr != nil || !bytes.Equal(candidateDecoded, decoded) {
			continue
		}
		if _, strictErr := base64.RawURLEncoding.Strict().DecodeString(candidate); strictErr != nil {
			variant = candidate
			break
		}
	}
	if variant == "" {
		t.Fatal("could not construct a non-canonical raw base64 cursor variant")
	}
	if _, err := decodeAuditCursor(variant); err == nil {
		t.Fatalf("non-canonical base64 cursor %q was accepted", variant)
	}
}

func TestParseAuditQueryRejectsCursorWhitespace(t *testing.T) {
	for _, cursor := range []string{
		" " + encodeAuditCursor(42),
		encodeAuditCursor(42) + "\t",
	} {
		if _, err := parseAuditQuery(url.Values{"cursor": {cursor}}); err == nil {
			t.Fatalf("cursor with surrounding whitespace %q was accepted", cursor)
		}
	}
}
