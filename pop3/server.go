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
