package password

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// TestHash_LengthLimit_U pins bcrypt's input limit, in bytes: the reason the
// requests validate passwords with maxbytes=72, not a character count.
func TestHash_LengthLimit_U(t *testing.T) {
	testCases := []struct {
		name    string
		plain   string
		wantErr bool
	}{
		{name: "72 plain letters (72 bytes) hash", plain: strings.Repeat("a", 72)},
		{name: "36 accented letters (72 bytes) hash", plain: strings.Repeat("é", 36)},
		{name: "73 bytes are refused", plain: strings.Repeat("a", 73), wantErr: true},
		{name: "40 accented letters (40 characters, 80 bytes) are refused", plain: strings.Repeat("é", 40), wantErr: true},
		{name: "36 decomposed é (108 bytes sent) hash: it is the NFC form (72 bytes) that is hashed",
			plain: strings.Repeat("e\u0301", 36)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			hash, err := Hash(tc.plain)
			if tc.wantErr {
				require.ErrorIs(t, err, bcrypt.ErrPasswordTooLong)
				require.Empty(t, hash)
				return
			}
			require.NoError(t, err)
			require.True(t, Check(hash, tc.plain))
		})
	}
}

func TestHashAndCheck_U(t *testing.T) {
	hash, err := Hash("S3cret!pass")
	require.NoError(t, err)
	require.NotEqual(t, "S3cret!pass", hash)

	testCases := []struct {
		name  string
		check func(hash, plain string) bool
		hash  string
		plain string
		want  bool
	}{
		{name: "matching password", check: Check, hash: hash, plain: "S3cret!pass", want: true},
		{name: "wrong password", check: Check, hash: hash, plain: "wrong", want: false},
		{name: "not a bcrypt hash", check: Check, hash: "plain-text", plain: "plain-text", want: false},
		{name: "an empty hash never matches", check: Check, hash: "", plain: "", want: false},
		{name: "VerifyOrWaste - matching password", check: okOrWaste, hash: hash, plain: "S3cret!pass", want: true},
		{name: "VerifyOrWaste - wrong password", check: okOrWaste, hash: hash, plain: "wrong", want: false},
		{name: "VerifyOrWaste - no hash still compares (against the dummy), never matches",
			check: okOrWaste, hash: "", plain: "go-boilerplate-timing-equalizer", want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.check(tc.hash, tc.plain))
		})
	}
}

func okOrWaste(hash, plain string) bool { return VerifyOrWaste(hash, plain).OK }

const (
	composed   = "caf\u00e9-Passw0rd"  // "é" as one code point (NFC), e.g. Windows
	decomposed = "cafe\u0301-Passw0rd" // "e" + a combining accent (NFD), e.g. macOS
)

// legacyHash is a hash made before decision 0019: bcrypt of the bytes as sent.
func legacyHash(t *testing.T, plain string) string {
	t.Helper()
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.MinCost)
	require.NoError(t, err)
	return string(b)
}

func TestVerify_NFC_U(t *testing.T) {
	newHash, err := Hash(decomposed) // registered from a Mac
	require.NoError(t, err)

	testCases := []struct {
		name  string
		hash  string
		plain string
		want  Match
	}{
		{name: "registered decomposed, signs in composed (another device): right, not legacy",
			hash: newHash, plain: composed, want: Match{OK: true}},
		{name: "registered decomposed, signs in decomposed: right, not legacy",
			hash: newHash, plain: decomposed, want: Match{OK: true}},
		{name: "a hash made before NFC from decomposed bytes matches the same bytes, flagged legacy (rehash it)",
			hash: legacyHash(t, decomposed), plain: decomposed, want: Match{OK: true, Legacy: true}},
		{name: "a hash made before NFC from composed bytes is already NFC: right, not legacy",
			hash: legacyHash(t, composed), plain: composed, want: Match{OK: true}},
		{name: "a hash made before NFC from decomposed bytes can't match the composed form (until rehashed)",
			hash: legacyHash(t, decomposed), plain: composed, want: Match{}},
		{name: "a wrong password, decomposed, is wrong", hash: newHash, plain: "cafe\u0301-wrong", want: Match{}},
		{name: "an empty hash never matches", hash: "", plain: decomposed, want: Match{}},
		{name: "a legacy 72-byte password plus one more character is wrong, though bcrypt alone would read only the first 72 bytes",
			hash: legacyHash(t, strings.Repeat("e\u0301", 24)), plain: strings.Repeat("e\u0301", 24) + "x", want: Match{}},
		{name: "72 bytes plus one more is wrong against an NFC hash too",
			hash: legacyHash(t, strings.Repeat("a", 72)), plain: strings.Repeat("a", 72) + "b", want: Match{}},
		{name: "a legacy password NFC makes longer than 72 bytes still matches as sent, but isn't flagged for a rehash bcrypt would refuse",
			hash: legacyHash(t, strings.Repeat("\u0958", 20)), plain: strings.Repeat("\u0958", 20), want: Match{OK: true}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, Verify(tc.hash, tc.plain))
		})
	}
}

// TestVerifyOrWaste_Comparisons_U pins the timing rule: an unknown email (no
// hash) spends as many bcrypt comparisons as a wrong password for a known one,
// the second (as-sent) comparison included when the input isn't NFC.
func TestVerifyOrWaste_Comparisons_U(t *testing.T) {
	hash, err := Hash(composed)
	require.NoError(t, err)

	testCases := []struct {
		name  string
		hash  string
		plain string
		want  int
	}{
		{name: "known account, wrong NFC password: one comparison", hash: hash, plain: "wrong", want: 1},
		{name: "unknown email, NFC password: one comparison", hash: "", plain: "wrong", want: 1},
		{name: "known account, wrong decomposed password: two (NFC, then as sent)", hash: hash, plain: "cafe\u0301-wrong", want: 2},
		{name: "unknown email, decomposed password: two as well", hash: "", plain: "cafe\u0301-wrong", want: 2},
		{name: "known account, right password: one", hash: hash, plain: decomposed, want: 1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			n := 0
			prev := compare
			compare = func(h, p []byte) error { n++; return prev(h, p) }
			defer func() { compare = prev }()

			VerifyOrWaste(tc.hash, tc.plain)
			require.Equal(t, tc.want, n)
		})
	}
}

func TestNormalize_U(t *testing.T) {
	testCases := []struct {
		name, in, want string
	}{
		{name: "decomposed \u00e9 becomes the composed one", in: "e\u0301", want: "\u00e9"},
		{name: "composed stays", in: "é", want: "é"},
		{name: "compatibility characters are kept (NFC, not NFKC): the fi ligature stays", in: "\ufb01", want: "\ufb01"},
		{name: "full-width digits stay", in: "\uff12", want: "\uff12"},
		{name: "ASCII is untouched", in: "Passw0rd!", want: "Passw0rd!"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, Normalize(tc.in))
		})
	}
}

func TestFitsNFC_U(t *testing.T) {
	testCases := []struct {
		name  string
		plain string
		n     int
		want  bool
	}{
		{name: "exactly n bytes in NFC fits", plain: strings.Repeat("e\u0301", 36), n: 72, want: true},
		{name: "one byte over doesn't", plain: strings.Repeat("e\u0301", 36) + "a", n: 72, want: false},
		{name: "the 3.5x shrink (\u1fbe + two marks -> \u0390) is counted exactly",
			plain: strings.Repeat("\u1fbe\u0308\u0301", 36), n: 72, want: true},
		{name: "a form NFC makes longer is counted as stored", plain: strings.Repeat("\u0958", 13), n: 72, want: false},
		{name: "empty fits", plain: "", n: 0, want: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, FitsNFC(tc.plain, tc.n))
			require.Equal(t, tc.want, len(Normalize(tc.plain)) <= tc.n, "agrees with Normalize")
		})
	}
}

// TestFitsNFC_StopsEarly_U pins the CPU guard: a huge input is refused after
// looking at about n bytes, not after normalizing all of it.
func TestFitsNFC_StopsEarly_U(t *testing.T) {
	huge := strings.Repeat("e\u0301", 1<<22) // 12 MB
	small := strings.Repeat("e\u0301", 1<<10)
	start := time.Now()
	for range 100 {
		require.False(t, FitsNFC(huge, 72))
	}
	hugeTook := time.Since(start)
	start = time.Now()
	for range 100 {
		require.False(t, FitsNFC(small, 72))
	}
	require.Less(t, hugeTook, 20*time.Millisecond+10*time.Since(start),
		"refusing 12 MB costs about what refusing 3 KB does")
}
