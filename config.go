package main

type Config struct {
	SMTPPort int
	POP3Port int

	// Hostname is the name of the MX server that is running.
	Hostname string

	Servers []Server
}

type Server struct {
	// Domain is the second component of a mail address: <local-part@domain.com>.
	Domain string

	TLSKeyPath  string
	TLSCertPath string

	// Password for the POP3 mailbox user, mailbox@domain.com.
	MailboxPassword string

	// Location to store the mail messages.
	MaildropPath string

	// Blacklisted addresses that should not accept mail.
	BlacklistedAddresses []string
}
