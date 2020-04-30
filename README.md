# mailpopbox

Mailpopbox is a combination delivery SMTP server and POP mailbox. The purpose is to act as a
catch-all delivery server for an MX domain. All messages that it receives are deposited into a
single mailbox, which can then be accessed using the POP3 protocol.

## TLS Support

TLS is recommended in production environments. To facilitate live-reloading of certificates, you can
send a running instance SIGHUP.

## Send-As SMTP

Since mailpopbox is designed as a catch-all mail server, it would be impractical to administer SMTP
accounts to enable replying from any address handled by the server. The SMTP server instead
provides a way to send messages from arbitrary addresses by authenticating as the mailbox@DOMAIN
user. Any valid SMTP MAIL FROM is supported after authentication, but mail clients will typically
use the mailbox@DOMAIN user or the From header. The SMTP server's feature is that if the message's
Subject header has a special "[sendas:ANYTHING]" string, the server will alter the From message
header to be from ANYTHING@DOMAIN.

Practically, this means configuring an outbound mail client to send mail as mailbox@DOMAIN and
authenticate to the SMTP server as such. And in order to change the sending address as perceived by
the recipient, edit the subject with [sendas:ADDRESS].

## RFCs

This server implements the following RFCs:

- [Post Office Protocol - Version 3, RFC 1939](https://tools.ietf.org/html/rfc1939)
- [Simple Mail Transfer Protocol, RFC 5321](https://tools.ietf.org/html/rfc5321)
- [Message Submission for Mail, RFC 6409](https://tools.ietf.org/html/rfc6409)
- [SMTP Service Extension for Secure SMTP over Transport Layer Security, RFC 3207](https://tools.ietf.org/html/rfc3207)
- [SMTP Service Extension for Authentication, RFC 2554](https://tools.ietf.org/html/rfc2554)
- [The PLAIN Simple Authentication and Security Layer (SASL) Mechanism, RFC 4616](https://tools.ietf.org/html/rfc4616)
- [POP3 Extension Mechanism, RFC 2449](https://tools.ietf.org/html/rfc2449)
