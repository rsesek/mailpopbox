package pop3

import (
	"io"
)

type Message interface {
	ID() int
	Size() int
}

type Mailbox interface {
	ListMessages() ([]Message, error)
	Retrieve(Message) (io.ReadCloser, error)
	Delete(Message) error
	Close() error
	Reset()
}

type PostOffice interface {
	Name() string
	OpenMailbox(user, pass string) (Mailbox, error)
}
