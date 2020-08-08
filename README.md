# mailpopbox

Mailpopbox is a single-user mail server with SMTP and POP facilities. Its purpose is to provide
privacy through anonymity by acting as a catch-all, wildcard email server for an entire domain name.
Any message addressed to an account at the configured domain will be deposited into a single
mailbox, which can then be accessed using the POP3 protocol.

The usage scenario is to configure your primary email provider (e.g.,
[Gmail](https://support.google.com/mail/answer/7104828)) or client to POP messages off the
server. Any time you need to provide an email address, you can give out
`arbitrary-string@domain.com`, allowing you to use site/service/person-specific email addresses.
Mail is collected into a single maildrop that can be POP'd out into your normal mailbox.

## Send-As SMTP

Mailpopbox also provides a way to reply to messages from an arbitrary email address at the domain.
Since mailpopbox is designed as a catch-all mail server, it would be impractical to administer SMTP
accounts to enable replying from any address handled by the server. Instead, the SMTP authenticates
a single *mailbox* user, and if the message's Subject header has a special `[sendas:ADDRESS]`
string, the server will alter the From message header to be from ADDRESS@DOMAIN.

## Installation

Installation requires a server capable of binding on port 25 for SMTP and 995 for POP3. A TLS
certificate is also required to have a secure connection for authenticating to the server. See the
[installation guide](docs/install.md) for a full set of steps.

## Building

Building mailpopbox only requires [Go](https://golang.org) and git. Clone the repository and type
`go build`. Cross-compilation is supported since the server is written in pure Go.

## Contributing

[![CI](https://github.com/rsesek/mailpopbox/workflows/CI/badge.svg)](https://github.com/rsesek/mailpopbox/actions?query=workflow%3ACI)

The canonical home for the repository is on
[src.bluestatic.org](https://src.bluestatic.org/?p=mailpopbox.git), but it is also hosted on
[Github](https://github.com/rsesek/mailpopbox) to enable collaboration.

Issues should be reported on the [Github issue tracker](https://github.com/rsesek/mailpopbox/issues).

Contributions are welcome. Please:
- Ensure tests pass and that new changes have adequate code coverage (`make coverage`)
- Code is formatted with `go fmt`
- Commit history is clean (one logical change per commit)
- Commit message is well-formatted

## RFCs

This server implements (partially) the following RFCs:

- [Post Office Protocol - Version 3, RFC 1939](https://tools.ietf.org/html/rfc1939)
- [Simple Mail Transfer Protocol, RFC 5321](https://tools.ietf.org/html/rfc5321)
- [Message Submission for Mail, RFC 6409](https://tools.ietf.org/html/rfc6409)
- [SMTP Service Extension for Secure SMTP over Transport Layer Security, RFC 3207](https://tools.ietf.org/html/rfc3207)
- [SMTP Service Extension for Authentication, RFC 2554](https://tools.ietf.org/html/rfc2554)
- [The PLAIN Simple Authentication and Security Layer (SASL) Mechanism, RFC 4616](https://tools.ietf.org/html/rfc4616)
- [Simple Mail Transfer Protocol (SMTP) Service Extension for Delivery Status Notifications (DSNs), RFC 3461](https://tools.ietf.org/html/rfc3461)
- [POP3 Extension Mechanism, RFC 2449](https://tools.ietf.org/html/rfc2449)
