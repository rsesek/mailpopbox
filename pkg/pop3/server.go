// mailpopbox
// Copyright 2020 Blue Static <https://www.bluestatic.org>
// This program is free software licensed under the GNU General Public License,
// version 3.0. The full text of the license can be found in LICENSE.txt.
// SPDX-License-Identifier: GPL-3.0-only

package pop3

import (
	"io"
)

type Message interface {
	UniqueID() string
	ID() int
	Size() int
	Deleted() bool
}

type Mailbox interface {
	ListMessages() ([]Message, error)
	GetMessage(int) Message
	Retrieve(Message) (io.ReadCloser, error)
	Delete(Message) error
	Close() error
	Reset()
}

type PostOffice interface {
	Name() string
	OpenMailbox(user, pass string) (Mailbox, error)
}
