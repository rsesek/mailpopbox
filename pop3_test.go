package main

import (
	"testing"
)

func TestReset(t *testing.T) {
	mbox := mailbox{
		messages: []message{
			{"msg1", 1, 4, false},
			{"msg2", 2, 4, false},
		},
	}

	msg := mbox.GetMessage(1)
	if msg == nil {
		t.Errorf("failed to GetMessage #1")
	}

	if msg.Deleted() {
		t.Errorf("message %v should not be deleted", msg)
	}

	if err := mbox.Delete(msg); err != nil {
		t.Error(err)
	}

	if !msg.Deleted() {
		t.Errorf("message %v should be deleted", msg)
	}

	mbox.Reset()

	if msg.Deleted() {
		t.Errorf("reset did not un-delete message %v", msg)
	}
}
