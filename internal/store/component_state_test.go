package store

import "testing"

func TestSetComponentStateWalksTreeAndAccessories(t *testing.T) {
	// Arrange
	s := New(0)
	s.AppendMessage(Message{
		ID:        5,
		ChannelID: 1,
		ComponentTree: []ComponentNode{{
			Kind: ComponentContainer,
			Children: []ComponentNode{{
				Kind: ComponentActionRow,
				Children: []ComponentNode{
					{Kind: ComponentButton, CustomID: "confirm"},
					{Kind: ComponentSelect, CustomID: "items"},
				},
			}},
			Accessory: &ComponentNode{Kind: ComponentButton, CustomID: "confirm"},
		}},
	})

	// Act
	if !s.SetComponentState(1, 5, "confirm", ComponentStatePending) {
		t.Fatal("SetComponentState did not find the message")
	}

	// Assert
	msg := s.Messages(1)[0]
	row := msg.ComponentTree[0].Children[0]
	if row.Children[0].State != ComponentStatePending {
		t.Fatalf("button state = %v, want pending", row.Children[0].State)
	}
	if row.Children[1].State != ComponentStateIdle {
		t.Fatalf("select state = %v, want untouched idle", row.Children[1].State)
	}
	if acc := msg.ComponentTree[0].Accessory; acc.State != ComponentStatePending {
		t.Fatalf("accessory state = %v, want pending", acc.State)
	}
}

func TestSetComponentStateRejectsEmptyCustomID(t *testing.T) {
	s := New(0)
	s.AppendMessage(Message{ID: 5, ChannelID: 1})
	if s.SetComponentState(1, 5, "", ComponentStatePending) {
		t.Fatal("empty custom id should not match anything")
	}
}
