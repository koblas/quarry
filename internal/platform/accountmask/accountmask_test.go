package accountmask_test

import (
	"testing"

	"github.com/koblas/quarry/internal/platform/accountmask"
	"github.com/stretchr/testify/assert"
)

func Test_Mask_keeps_the_last_four_digits(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "no digits", in: "checking", want: "checking"},
		{name: "one digit", in: "7", want: "7"},
		{name: "four digits", in: "5678", want: "5678"},
		{name: "five digits hide the first", in: "12345", want: "*2345"},
		{name: "eight digits hide four", in: "12345678", want: "****5678"},
		{name: "twelve digits with separators keep the separators", in: "1234 5678 9012", want: "**** **** 9012"},
		{name: "four digits among words are a year, not a number", in: "RBC 2019 TFSA", want: "RBC 2019 TFSA"},
		{name: "digits are counted across the whole string", in: "RBC 2019 TFSA 5678", want: "RBC **** TFSA 5678"},
		{name: "non-ASCII digits count and are replaced", in: "١٢٣٤٥٦٧٨", want: "****٥٦٧٨"},
		{name: "multibyte letters are kept", in: "café 12345", want: "café *2345"},
		{name: "empty", in: "", want: ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, accountmask.Mask(c.in))
		})
	}
}

func Test_Mask_leaves_an_id_of_the_exact_form_acct_digits_unchanged(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{name: "eight digits", in: "acct-12345678"},
		{name: "one digit", in: "acct-1"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.in, accountmask.Mask(c.in))
		})
	}
}

func Test_Mask_masks_text_that_only_resembles_an_acct_id(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "capital A", in: "Acct-12345678", want: "Acct-****5678"},
		{name: "trailing letter", in: "acct-12345678x", want: "acct-****5678x"},
		{name: "leading space", in: " acct-12345678", want: " acct-****5678"},
		{name: "quoted", in: `"acct-12345678"`, want: `"acct-****5678"`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, accountmask.Mask(c.in))
		})
	}
}

func Test_Mask_is_idempotent(t *testing.T) {
	once := accountmask.Mask("1234 5678 9012")

	assert.Equal(t, once, accountmask.Mask(once))
}
