# mailpopbox

Mailpopbox is a combination delivery SMTP server and POP mailbox. The purpose is to act as a
catch-all delivery server for an MX domain. All messages that it receives are deposited into a
single mailbox, which can then be accessed using the POP3 protocol.

## TLS Support

TLS is recommended in production environments. To facilitate live-reloading of certificates, you can
send a running instance SIGHUP.

## RFCs

This server implements the following RFCs:

- [Post Office Protocol - Version 3, RFC 1939](https://tools.ietf.org/html/rfc1939)
- [Simple Mail Transfer Protocol, RFC 5321](https://tools.ietf.org/html/rfc5321)
- [SMTP Service Extension for Secure SMTP over Transport Layer Security, RFC 3207](https://tools.ietf.org/html/rfc3207)
- [SMTP Service Extension for Authentication, RFC 2554](https://tools.ietf.org/html/rfc2554)
- [The PLAIN Simple Authentication and Security Layer (SASL) Mechanism, RFC 4616](https://tools.ietf.org/html/rfc4616)
- [POP3 Extension Mechanism, RFC 2449](https://tools.ietf.org/html/rfc2449)
