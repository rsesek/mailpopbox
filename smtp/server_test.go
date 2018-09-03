package smtp

import (
	"net/mail"
	"testing"
)

func TestDomainForAddress(t *testing.T) {
	cases := []struct{
		address, domain string
	}{
		{"foo@bar.com", "bar.com"},
		{"abc", ""},
		{"abc@one.two.three.four.net", "one.two.three.four.net"},
	}
	for i, c := range cases {
		actual := DomainForAddress(mail.Address{Address: c.address})
		if actual != c.domain {
			t.Errorf("case %d, got %q, expected %q", i, actual, c.domain)
		}
	}
}
