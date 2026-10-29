package tests

import (
	"testing"

	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
	chatstruct "github.com/caliniya/caf/core/Chat/ChatStruct"
)

func TestGetMessagesUsesSelectedBranch(t *testing.T) {
	root := chatstruct.CreateChatNode(apiImpl.ChatMessage{Role: "user", Content: "question"})
	first := chatstruct.CreateChatNode(apiImpl.ChatMessage{Role: "assistant", Content: "first answer"})
	second := chatstruct.CreateChatNode(apiImpl.ChatMessage{Role: "assistant", Content: "second answer"})
	root.AddNext(first)
	root.AddNext(second)

	messages, err := root.GetMessages()
	if err != nil {
		t.Fatalf("GetMessages returned error: %v", err)
	}
	if len(messages) != 2 || messages[1].Content != "second answer" {
		t.Fatalf("unexpected current branch messages: %#v", messages)
	}

	if err := root.SelectNext(0); err != nil {
		t.Fatalf("SelectNext returned error: %v", err)
	}
	messages, err = root.GetMessages()
	if err != nil {
		t.Fatalf("GetMessages after branch selection returned error: %v", err)
	}
	if messages[1].Content != "first answer" {
		t.Fatalf("expected first branch, got %q", messages[1].Content)
	}
}

func TestGetMessagesRejectsInvalidChoice(t *testing.T) {
	root := chatstruct.CreateChatNode()
	root.Next = []*chatstruct.ChatNode{chatstruct.CreateChatNode()}
	root.Choice = 2

	if _, err := root.GetMessages(); err == nil {
		t.Fatal("expected invalid branch choice to return an error")
	}
	if root.GetNext() != nil {
		t.Fatal("GetNext should return nil for an invalid choice")
	}
}

func TestGetMessagesRejectsCycle(t *testing.T) {
	root := chatstruct.CreateChatNode()
	root.AddNext(root)

	if _, err := root.GetMessages(); err == nil {
		t.Fatal("expected a cycle to return an error")
	}
}
