// mailpopbox
// Copyright 2020 Blue Static <https://www.bluestatic.org>
// This program is free software licensed under the GNU General Public License,
// version 3.0. The full text of the license can be found in LICENSE.txt.
// SPDX-License-Identifier: GPL-3.0-only

package smtp

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"mime/multipart"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"time"

	"go.uber.org/zap"
)

func RelayMessage(server Server, env Envelope, log *zap.Logger) {
	for _, rcptTo := range env.RcptTo {
		sendLog := log.With(zap.String("address", rcptTo.Address))

		domain := DomainForAddress(rcptTo)
		mx, err := net.LookupMX(domain)
		if err != nil || len(mx) < 1 {
			deliverRelayFailure(server, env, log, rcptTo.Address, "failed to lookup MX records", err)
			return
		}
		host := mx[0].Host + ":25"
		relayMessageToHost(server, env, sendLog, rcptTo.Address, host)
	}
}

func relayMessageToHost(server Server, env Envelope, log *zap.Logger, to, host string) {
	from := env.MailFrom.Address
	log = log.With(zap.String("host", host))

	c, err := smtp.Dial(host)
	if err != nil {
		// TODO - retry, or look at other MX records
		deliverRelayFailure(server, env, log, to, "failed to dial host", err)
		return
	}
	defer c.Quit()

	if err = c.Hello(server.Name()); err != nil {
		deliverRelayFailure(server, env, log, to, "failed to HELO", err)
		return
	}

	if hasTls, _ := c.Extension("STARTTLS"); hasTls {
		config := &tls.Config{ServerName: host}
		if err = c.StartTLS(config); err != nil {
			deliverRelayFailure(server, env, log, to, "failed to STARTTLS", err)
			return
		}
	}

	if err = c.Mail(from); err != nil {
		deliverRelayFailure(server, env, log, to, "failed MAIL FROM", err)
		return
	}

	if err = c.Rcpt(to); err != nil {
		deliverRelayFailure(server, env, log, to, "failed to RCPT TO", err)
		return
	}

	wc, err := c.Data()
	if err != nil {
		deliverRelayFailure(server, env, log, to, "failed to DATA", err)
		return
	}

	_, err = wc.Write(env.Data)
	if err != nil {
		wc.Close()
		deliverRelayFailure(server, env, log, to, "failed to write DATA", err)
		return
	}

	if err = wc.Close(); err != nil {
		deliverRelayFailure(server, env, log, to, "failed to close DATA", err)
		return
	}
}

// deliverRelayFailure logs and generates a delivery status notification. It
// writes to |log| the |errorStr| and |sendErr|, as well as preparing a new
// message, based of |env|, delivered to |server| that reports error
// information about the attempted delivery.
func deliverRelayFailure(server Server, env Envelope, log *zap.Logger, to, errorStr string, sendErr error) {
	log.Error(errorStr, zap.Error(sendErr))

	buf := &bytes.Buffer{}
	mw := multipart.NewWriter(buf)

	now := time.Now()

	failure := Envelope{
		MailFrom: mail.Address{"mailpopbox", "mailbox@" + DomainForAddress(env.MailFrom)},
		RcptTo:   []mail.Address{env.MailFrom},
		ID:       generateEnvelopeId("f", now),
		Received: now,
	}

	fmt.Fprintf(buf, "From: %s\n", failure.MailFrom.String())
	fmt.Fprintf(buf, "To: %s\n", failure.RcptTo[0].String())
	fmt.Fprintf(buf, "Subject: Delivery Status Notification (Failure)\n")
	fmt.Fprintf(buf, "X-Failed-Recipients: %s\n", to)
	fmt.Fprintf(buf, "Message-ID: %s\n", failure.ID)
	fmt.Fprintf(buf, "Date: %s\n", now.Format(time.RFC1123Z))
	fmt.Fprintf(buf, "Content-Type: multipart/report; boundary=%s; report-type=delivery-status\n\n", mw.Boundary())

	tw, err := mw.CreatePart(textproto.MIMEHeader{
		"Content-Type": []string{"text/plain; charset=UTF-8"},
	})
	if err != nil {
		log.Error("failed to create multipart 0", zap.Error(err))
		return
	}
	fmt.Fprintf(tw, "* * * Delivery Failure * * *\n\n")
	fmt.Fprintf(tw, "The server failed to relay the message:\n\n%s:\n%s\n", errorStr, sendErr.Error())

	sw, err := mw.CreatePart(textproto.MIMEHeader{
		"Content-Type": []string{"delivery-status"},
	})
	if err != nil {
		log.Error("failed to create multipart 1", zap.Error(err))
		return
	}
	fmt.Fprintf(sw, "Original-Envelope-ID: %s\n", env.ID)
	fmt.Fprintf(sw, "Reporting-UA: %s\n", env.EHLO)
	if env.RemoteAddr != nil {
		rhosts, err := net.LookupAddr(env.RemoteAddr.String())
		if err == nil {
			fmt.Fprintf(sw, "Reporting-MTA: %s\n", rhosts[0])
		}
		fmt.Fprintf(sw, "X-Remote-Address: %s\n", env.RemoteAddr)
	}
	fmt.Fprintf(sw, "Date: %s\n", env.Received.Format(time.RFC1123Z))

	ocw, err := mw.CreatePart(textproto.MIMEHeader{
		"Content-Type": []string{"message/rfc822"},
	})
	if err != nil {
		log.Error("failed to create multipart 2", zap.Error(err))
		return
	}

	ocw.Write(env.Data)

	mw.Close()

	failure.Data = buf.Bytes()
	server.OnMessageDelivered(failure)
}
