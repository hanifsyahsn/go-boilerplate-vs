package validation

import (
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/require"
)

func TestNFCMaxBytes_U(t *testing.T) {
	type limited struct {
		Value string `validate:"nfcmaxbytes=72"`
	}
	type badParam struct {
		Value string `validate:"nfcmaxbytes=abc"`
	}
	type negativeParam struct {
		Value string `validate:"nfcmaxbytes=-1"`
	}
	type byteSlice struct {
		Value []byte `validate:"nfcmaxbytes=72"`
	}
	type pointer struct {
		Value *string `validate:"omitempty,nfcmaxbytes=72"`
	}
	const decomposedE = "e\u0301" // "é" as "e" + a combining accent: 3 bytes, NFC "é" is 2
	const qa = "\u0958"           // क़: 3 bytes, NFC makes it क + ़ (6 bytes)
	short, long := "short", strings.Repeat("é", 37)

	v := validator.New()
	require.NoError(t, v.RegisterValidation("nfcmaxbytes", NFCMaxBytes))

	testCases := []struct {
		name    string
		value   any
		wantErr bool
	}{
		{name: "72 plain letters pass", value: limited{strings.Repeat("a", 72)}},
		{name: "73 plain letters fail", value: limited{strings.Repeat("a", 73)}, wantErr: true},
		{name: "36 decomposed é (108 bytes sent, 72 in NFC) pass: a Mac's password isn't refused",
			value: limited{strings.Repeat(decomposedE, 36)}},
		{name: "37 decomposed é (74 bytes in NFC) fail", value: limited{strings.Repeat(decomposedE, 37)}, wantErr: true},
		{name: "12 क़ (36 bytes sent, 72 in NFC) pass", value: limited{strings.Repeat(qa, 12)}},
		{name: "13 क़ (39 bytes sent, 78 in NFC) fail: counted as stored, never reaching bcrypt's 500",
			value: limited{strings.Repeat(qa, 13)}, wantErr: true},
		{name: "36 \u1fbe + diaeresis + acute (252 bytes sent, 72 in NFC: 3.5x) pass",
			value: limited{strings.Repeat("\u1fbe\u0308\u0301", 36)}},
		{name: "24 Hangul jamo syllables (216 bytes sent, 72 in NFC) pass",
			value: limited{strings.Repeat("\u1100\u1161\u11a8", 24)}},
		{name: "a 1 MB password fails", value: limited{strings.Repeat("e\u0301", 1<<20/3)}, wantErr: true},
		{name: "a param that isn't a number fails every value", value: badParam{"a"}, wantErr: true},
		{name: "a negative param fails every value", value: negativeParam{""}, wantErr: true},
		{name: "a field that isn't a string fails every value", value: byteSlice{[]byte("a")}, wantErr: true},
		{name: "a *string is checked through the pointer: short passes", value: pointer{&short}},
		{name: "a *string is checked through the pointer: over 72 bytes fails", value: pointer{&long}, wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := v.Struct(tc.value)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestCheckMaxBytes_U(t *testing.T) {
	type limited struct {
		Value string `validate:"checkmaxbytes=72"`
	}
	type badParam struct {
		Value string `validate:"checkmaxbytes=x"`
	}
	type byteSlice struct {
		Value []byte `validate:"checkmaxbytes=72"`
	}
	const decomposedE = "e\u0301"
	const qa = "\u0958"

	v := validator.New()
	require.NoError(t, v.RegisterValidation("checkmaxbytes", CheckMaxBytes))

	testCases := []struct {
		name    string
		value   any
		wantErr bool
	}{
		{name: "72 plain letters pass", value: limited{strings.Repeat("a", 72)}},
		{name: "73 plain letters fail", value: limited{strings.Repeat("a", 73)}, wantErr: true},
		{name: "36 decomposed é (108 sent, 72 in NFC) pass: a Mac's login isn't refused",
			value: limited{strings.Repeat(decomposedE, 36)}},
		{name: "20 क़ (60 sent, 120 in NFC) pass: a hash made before 0019 from them can still be checked",
			value: limited{strings.Repeat(qa, 20)}},
		{name: "24 Hangul jamo syllables (216 bytes sent, 72 in NFC) pass",
			value: limited{strings.Repeat("\u1100\u1161\u11a8", 24)}},
		{name: "36 \u1fbe + diaeresis + acute (252 bytes sent, 72 in NFC) pass",
			value: limited{strings.Repeat("\u1fbe\u0308\u0301", 36)}},
		{name: "a 1 MB password fails", value: limited{strings.Repeat("e\u0301", 1<<20/3)}, wantErr: true},
		{name: "37 decomposed é (111 sent, 74 in NFC) fail: neither form fits",
			value: limited{strings.Repeat(decomposedE, 37)}, wantErr: true},
		{name: "a param that isn't a number fails every value", value: badParam{"a"}, wantErr: true},
		{name: "a field that isn't a string fails every value", value: byteSlice{[]byte("a")}, wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := v.Struct(tc.value)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
