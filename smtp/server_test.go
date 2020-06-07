// mailpopbox
// Copyright 2020 Blue Static <https://www.bluestatic.org>
// This program is free software licensed under the GNU General Public License,
// version 3.0. The full text of the license can be found in LICENSE.txt.
// SPDX-License-Identifier: GPL-3.0-only

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
