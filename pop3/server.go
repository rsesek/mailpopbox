package pop3

import (
	"io"
)

type Message interface {
	ID() int
	Size() int
	Deleted() bool
}

type Mailbox interface {
	ListMessages() ([]Message, error)
	Retrieve(int) (io.ReadCloser, error)
	Delete(int) error
	Close() error
	Reset()
}

type PostOffice interface {
	Name() string
	OpenMailbox(user, pass string) (Mailbox, error)
}
