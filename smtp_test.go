package main

import (
	"testing"
)

var testConfig = Config{
	Servers: []Server{
		Server{
			Domain:          "domain1.net",
			MailboxPassword: "d1",
		},
		Server{
			Domain:          "domain2.xyz",
			MailboxPassword: "d2",
		},
	},
}

func TestAuthenticate(t *testing.T) {
	server := smtpServer{config: testConfig}

	authTests := []struct {
		authz, authc, passwd string
		ok                   bool
	}{
		{"foo@domain1.net", "mailbox@domain1.net", "d1", true},
		{"", "mailbox@domain1.net", "d1", true},
		{"foo@domain2.xyz", "mailbox@domain1.xyz", "d1", false},
		{"foo@domain2.xyz", "mailbox@domain1.xyz", "d2", false},
		{"foo@domain2.xyz", "mailbox@domain2.xyz", "d2", true},
		{"invalid", "mailbox@domain2.xyz", "d2", false},
		{"", "mailbox@domain2.xyz", "d2", true},
		{"", "", "", false},
	}

	for i, test := range authTests {
		actual := server.Authenticate(test.authz, test.authc, test.passwd)
		if actual != test.ok {
			t.Errorf("Test %d, got %v, expected %v", i, actual, test.ok)
		}
	}
}
