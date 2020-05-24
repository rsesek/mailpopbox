// mailpopbox
// Copyright 2020 Blue Static <https://www.bluestatic.org>
// This program is free software licensed under the GNU General Public License,
// version 3.0. The full text of the license can be found in LICENSE.txt.
// SPDX-License-Identifier: GPL-3.0-only

package smtp

import (
	"crypto/tls"
	"net"
	"net/smtp"

	"go.uber.org/zap"
)

func RelayMessage(server Server, env Envelope, log *zap.Logger) {
	for _, rcptTo := range env.RcptTo {
		sendLog := log.With(zap.String("address", rcptTo.Address))

		domain := DomainForAddress(rcptTo)
		mx, err := net.LookupMX(domain)
		if err != nil || len(mx) < 1 {
			sendLog.Error("failed to lookup MX records",
				zap.Error(err))
			deliverRelayFailure(env, err)
			return
		}
		host := mx[0].Host + ":25"
		relayMessageToHost(server, env, sendLog, rcptTo.Address, host)
	}
}

func relayMessageToHost(server Server, env Envelope, log *zap.Logger, to, host string) {
	from := env.MailFrom.Address

	c, err := smtp.Dial(host)
	if err != nil {
		// TODO - retry, or look at other MX records
		log.Error("failed to dial host",
			zap.String("host", host),
			zap.Error(err))
		deliverRelayFailure(env, err)
		return
	}
	defer c.Quit()

	log = log.With(zap.String("host", host))

	if err = c.Hello(server.Name()); err != nil {
		log.Error("failed to HELO", zap.Error(err))
		deliverRelayFailure(env, err)
		return
	}

	if hasTls, _ := c.Extension("STARTTLS"); hasTls {
		config := &tls.Config{ServerName: host}
		if err = c.StartTLS(config); err != nil {
			log.Error("failed to STARTTLS", zap.Error(err))
			deliverRelayFailure(env, err)
			return
		}
	}

	if err = c.Mail(from); err != nil {
		log.Error("failed MAIL FROM", zap.Error(err))
		deliverRelayFailure(env, err)
		return
	}

	if err = c.Rcpt(to); err != nil {
		log.Error("failed to RCPT TO", zap.Error(err))
		deliverRelayFailure(env, err)
		return
	}

	wc, err := c.Data()
	if err != nil {
		log.Error("failed to DATA", zap.Error(err))
		deliverRelayFailure(env, err)
		return
	}

	_, err = wc.Write(env.Data)
	if err != nil {
		wc.Close()
		log.Error("failed to write DATA", zap.Error(err))
		deliverRelayFailure(env, err)
		return
	}

	if err = wc.Close(); err != nil {
		log.Error("failed to close DATA", zap.Error(err))
		deliverRelayFailure(env, err)
		return
	}
}

func deliverRelayFailure(env Envelope, err error) {
	// TODO: constructo a delivery status notification
}
