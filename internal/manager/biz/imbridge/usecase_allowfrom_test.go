package imbridge

import (
	"reflect"
	"testing"
)

func TestParseAllowFrom(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{"empty", "", nil},
		{"single", "12345", []string{"12345"}},
		{"comma+space", "111, 222 , 333", []string{"111", "222", "333"}},
		{"newline+semicolon", "111\n222;333", []string{"111", "222", "333"}},
		{"strip prefixes", "telegram:111, tg:222, 333", []string{"111", "222", "333"}},
		{"dedup preserves order", "222,111,222,111", []string{"222", "111"}},
		// Security boundary: negative IDs (group/supergroup chat IDs) and
		// non-numeric junk must NOT become allowed senders.
		{"reject negative chat ids", "-1001234567890, 111", []string{"111"}},
		{"reject non-numeric", "abc, 12a, , 111", []string{"111"}},
		{"all junk -> empty (deny all)", "-1, abc, *, tg:", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseAllowFrom(tc.raw)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ParseAllowFrom(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

// A Telegram app with no resolvable allowlist must fail validation — an
// empty allowlist on a publicly-discoverable bot is the exact exposure
// ADR-031 closes.
func TestValidateTelegramRequiresAllowFrom(t *testing.T) {
	base := AppInput{Provider: "telegram", Mode: "stream", Name: "tg", AppID: "bot", AppSecret: "tok"}

	in := base
	if err := in.validate(); err == nil {
		t.Error("telegram with empty allow_from should be rejected")
	}

	in = base
	in.AllowFrom = "-1, garbage" // nothing valid resolves
	if err := in.validate(); err == nil {
		t.Error("telegram with no VALID allow_from id should be rejected")
	}

	in = base
	in.AllowFrom = "tg:8211893274"
	if err := in.validate(); err != nil {
		t.Errorf("telegram with a valid allow_from should pass: %v", err)
	}
	if in.AllowFrom != "8211893274" {
		t.Errorf("allow_from should be canonicalized to %q, got %q", "8211893274", in.AllowFrom)
	}

	// Telegram is stream-only.
	in = base
	in.Mode = "webhook"
	in.AllowFrom = "111"
	if err := in.validate(); err == nil {
		t.Error("telegram webhook mode should be rejected")
	}
}
