package collect

import (
	"errors"
	"fmt"
	"testing"

	"github.com/kibomibo/sshmon/internal/sshx"
)

// Sentinel'ы collect должны совпадать с транспортными в обе стороны: UI матчит
// ошибку, пришедшую из sshx, по collect.Err*, и наоборот.
func TestSentinelErrorsMatchTransport(t *testing.T) {
	cases := []struct {
		name      string
		collectiv error
		transport error
	}{
		{"passphrase required", ErrPassphraseRequired, sshx.ErrPassphraseRequired},
		{"invalid passphrase", ErrInvalidPassphrase, sshx.ErrInvalidPassphrase},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !errors.Is(tc.transport, tc.collectiv) {
				t.Fatalf("errors.Is(транспортная, collect.%v) = false", tc.collectiv)
			}
			if !errors.Is(tc.collectiv, tc.transport) {
				t.Fatalf("errors.Is(collect.%v, транспортная) = false", tc.collectiv)
			}
			wrapped := fmt.Errorf("реконнект web: %w", tc.transport)
			if !errors.Is(wrapped, tc.collectiv) {
				t.Fatalf("обёрнутая транспортная ошибка не матчится по collect.%v", tc.collectiv)
			}
		})
	}
	if errors.Is(ErrPassphraseRequired, ErrInvalidPassphrase) {
		t.Fatal("разные sentinel'ы не должны совпадать")
	}
}
