package main

import (
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// newTestMessageStore creates a MessageStore backed by an in-memory SQLite DB
func newTestMessageStore(t *testing.T) *MessageStore {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS chats (
			jid TEXT PRIMARY KEY,
			name TEXT,
			last_message_time TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS messages (
			id TEXT,
			chat_jid TEXT,
			sender TEXT,
			content TEXT,
			timestamp TIMESTAMP,
			is_from_me BOOLEAN,
			media_type TEXT,
			filename TEXT,
			url TEXT,
			media_key BLOB,
			file_sha256 BLOB,
			file_enc_sha256 BLOB,
			file_length INTEGER,
			PRIMARY KEY (id, chat_jid),
			FOREIGN KEY (chat_jid) REFERENCES chats(jid)
		);
	`)
	if err != nil {
		t.Fatalf("create tables: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &MessageStore{db: db}
}

func TestStoreOutboundMessage(t *testing.T) {
	store := newTestMessageStore(t)

	chatJID := "5511999999999@s.whatsapp.net"
	sender := "5511888888888"
	msgID := "TEST_MSG_001"
	content := "Hello from MCP"
	ts := time.Now()

	// Store the chat first (mirrors what the fix does)
	if err := store.StoreChat(chatJID, chatJID, ts); err != nil {
		t.Fatalf("StoreChat: %v", err)
	}

	// Store outbound message with is_from_me = true (the fix)
	if err := store.StoreMessage(msgID, chatJID, sender, content, ts, true, "", "", "", nil, nil, nil, 0); err != nil {
		t.Fatalf("StoreMessage: %v", err)
	}

	// Verify message can be retrieved via GetMessages (same as list_messages)
	msgs, err := store.GetMessages(chatJID, 10)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != content {
		t.Errorf("content = %q, want %q", msgs[0].Content, content)
	}
	if !msgs[0].IsFromMe {
		t.Error("expected IsFromMe = true")
	}
	if msgs[0].Sender != sender {
		t.Errorf("sender = %q, want %q", msgs[0].Sender, sender)
	}
}

func TestStoreOutboundMediaMessage(t *testing.T) {
	store := newTestMessageStore(t)

	chatJID := "5511999999999@s.whatsapp.net"
	sender := "5511888888888"
	msgID := "TEST_MSG_002"
	content := "Check this photo"
	ts := time.Now()

	if err := store.StoreChat(chatJID, chatJID, ts); err != nil {
		t.Fatalf("StoreChat: %v", err)
	}

	// Store outbound media message
	if err := store.StoreMessage(msgID, chatJID, sender, content, ts, true, "image", "photo.jpg", "", nil, nil, nil, 0); err != nil {
		t.Fatalf("StoreMessage: %v", err)
	}

	msgs, err := store.GetMessages(chatJID, 10)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].MediaType != "image" {
		t.Errorf("media_type = %q, want %q", msgs[0].MediaType, "image")
	}
	if msgs[0].Filename != "photo.jpg" {
		t.Errorf("filename = %q, want %q", msgs[0].Filename, "photo.jpg")
	}
}

func TestOutboundMessageAppearsAlongsideInbound(t *testing.T) {
	store := newTestMessageStore(t)

	chatJID := "5511999999999@s.whatsapp.net"
	ts := time.Now()

	if err := store.StoreChat(chatJID, chatJID, ts); err != nil {
		t.Fatalf("StoreChat: %v", err)
	}

	// Simulate an incoming message (already worked before the fix)
	if err := store.StoreMessage("INCOMING_1", chatJID, "5511999999999", "Hey there", ts.Add(-time.Minute), false, "", "", "", nil, nil, nil, 0); err != nil {
		t.Fatalf("StoreMessage incoming: %v", err)
	}

	// Simulate an outbound message (the fix)
	if err := store.StoreMessage("OUTBOUND_1", chatJID, "5511888888888", "Hi back!", ts, true, "", "", "", nil, nil, nil, 0); err != nil {
		t.Fatalf("StoreMessage outbound: %v", err)
	}

	msgs, err := store.GetMessages(chatJID, 10)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	// Verify we have both directions
	var hasInbound, hasOutbound bool
	for _, m := range msgs {
		if !m.IsFromMe {
			hasInbound = true
		}
		if m.IsFromMe {
			hasOutbound = true
		}
	}
	if !hasInbound || !hasOutbound {
		t.Errorf("expected both inbound and outbound messages; inbound=%v outbound=%v", hasInbound, hasOutbound)
	}
}

func TestEmptyMessageNotStored(t *testing.T) {
	store := newTestMessageStore(t)

	chatJID := "5511999999999@s.whatsapp.net"
	ts := time.Now()

	if err := store.StoreChat(chatJID, chatJID, ts); err != nil {
		t.Fatalf("StoreChat: %v", err)
	}

	// Empty content and no media — StoreMessage should silently skip
	if err := store.StoreMessage("EMPTY_1", chatJID, "sender", "", ts, true, "", "", "", nil, nil, nil, 0); err != nil {
		t.Fatalf("StoreMessage: %v", err)
	}

	msgs, err := store.GetMessages(chatJID, 10)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages for empty content, got %d", len(msgs))
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
