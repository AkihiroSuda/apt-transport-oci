package apt

import (
	"strings"
	"testing"
)

// TestParseConfig reads Config-Item as apt's method and DeQuoteString do.
func TestParseConfig(t *testing.T) {
	input := "601 Configuration\n" +
		"Config-Item: APT::Architecture=amd64\n" +
		"Config-Item: Acquire::oci::Note=a%20b%3dc\n" +
		"Config-Item: Acquire::oci::Bad=100%zz%4\n" +
		"Config-Item: Acquire::oci::Empty=\n" +
		"\n"
	msg, err := NewMessageReader(strings.NewReader(input)).ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	config := ParseConfig(msg)
	for key, want := range map[string]string{
		"apt::ARCHITECTURE":     "amd64",
		"Acquire::oci::Note":    "a b=c",
		"Acquire::oci::Bad":     "100%zz%4", // kept, as DeQuoteString
		"Acquire::oci::Empty":   "default",  // empty is unset, as Find
		"Acquire::oci::Missing": "default",
	} {
		if got := config.Find(key, "default"); got != want {
			t.Errorf("Find(%q): expected %q, got %q", key, want, got)
		}
	}
}

// TestConfigFindI reads the leading integer as apt's strtol with base 0; each
// value was checked against libc's strtol.
func TestConfigFindI(t *testing.T) {
	for value, want := range map[string]int{
		"30": 30, " 30": 30, "+30": 30, "-5": -5,
		"60s": 60, "0x1e": 30, "0X1E": 30, "036": 30, "08": 0,
		"": -1, "soon": -1, "s60": -1, "-": -1,
	} {
		if got := (Config{"acquire::oci::timeout": value}).FindI("Acquire::oci::Timeout", -1); got != want {
			t.Errorf("FindI(%q): expected %d, got %d", value, want, got)
		}
	}
}

// TestConfigFindB reads as apt's StringToBool.
func TestConfigFindB(t *testing.T) {
	for value, want := range map[string]bool{
		"1": true, "0x1": true, "yes": true, "True": true, "with": true, "on": true, "enable": true,
		"0": false, "no": false, "FALSE": false, "without": false, "off": false, "disable": false,
	} {
		if got := (Config{"debug::nodropprivs": value}).FindB("Debug::NoDropPrivs", !want); got != want {
			t.Errorf("FindB(%q): expected %v, got %v", value, want, got)
		}
	}
	// not wholly 0 or 1, nor a known word
	for _, value := range []string{"", "2", "1s", "maybe"} {
		for _, def := range []bool{false, true} {
			if got := (Config{"debug::nodropprivs": value}).FindB("Debug::NoDropPrivs", def); got != def {
				t.Errorf("FindB(%q) with default %v: got %v", value, def, got)
			}
		}
	}
}

func TestReadMessageTruncated(t *testing.T) {
	r := NewMessageReader(strings.NewReader("600 URI Acquire\nURI: oci://example/x:y/z\n"))
	if msg, err := r.ReadMessage(); err == nil {
		t.Fatalf("expected an error, got %+v", msg)
	}
}
