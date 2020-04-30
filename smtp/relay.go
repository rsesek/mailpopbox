// mailpopbox
// Copyright 2020 Blue Static <https://www.bluestatic.org>
// This program is free software licensed under the GNU General Public License,
// version 3.0. The full text of the license can be found in LICENSE.txt.
// SPDX-License-Identifier: GPL-3.0-only

package smtp

import (
	"net"
	"net/smtp"

	"github.com/uber-go/zap"
)

func RelayMessage(env Envelope, log zap.Logger) {
	for _, rcptTo := range env.RcptTo {
		domain := DomainForAddress(rcptTo)
		mx, err := net.LookupMX(domain)
		if err != nil || len(mx) < 1 {
			log.Error("failed to lookup MX records",
				zap.String("address", rcptTo.Address),
				zap.Error(err))
			deliverRelayFailure(env, err)
			return
		}

		to := []string{rcptTo.Address}
		from := env.MailFrom.Address
		host := mx[0].Host + ":25"

		log.Info("relay message",
			zap.String("to", to[0]),
			zap.String("from", from),
			zap.String("server", host))
		err = smtp.SendMail(host, nil, from, to, env.Data)
		if err != nil {
			log.Error("failed to relay message",
				zap.String("address", rcptTo.Address),
				zap.Error(err))
			deliverRelayFailure(env, err)
			return
		}
	}
}

func deliverRelayFailure(env Envelope, err error) {
	// TODO: constructo a delivery status notification
}
