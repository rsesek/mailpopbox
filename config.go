package main

import (
	"crypto/tls"
)

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

func (c Config) GetTLSConfig() (*tls.Config, error) {
	certs := make([]tls.Certificate, 0, len(c.Servers))
	for _, server := range c.Servers {
		if server.TLSCertPath == "" {
			continue
		}

		cert, err := tls.LoadX509KeyPair(server.TLSCertPath, server.TLSKeyPath)
		if err != nil {
			return nil, err
		}
		certs = append(certs, cert)
	}

	if len(certs) == 0 {
		return nil, nil
	}

	config := &tls.Config{
		Certificates: certs,
	}
	config.BuildNameToCertificate()
	return config, nil
}
