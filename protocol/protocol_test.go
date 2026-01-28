package protocol

import (
	"testing"
)

func TestParseSub(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantCmd byte
		wantSub string
		wantErr bool
	}{
		{
			name:    "valid subscribe",
			input:   NewMessageCmd(CmdSub, "foo.bar", nil).Encode(),
			wantCmd: CmdSub,
			wantSub: "foo.bar",
			wantErr: false,
		},
		{
			name:    "subscribe with wildcard",
			input:   NewMessageCmd(CmdSub, "foo.*", nil).Encode(),
			wantCmd: CmdSub,
			wantSub: "foo.*",
			wantErr: false,
		},
		{
			name:    "invalid subscribe - no subject",
			input:   []byte{0x00}, // invalid data
			wantCmd: CmdSub,
			wantSub: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, _, err := ParseWithRemaining(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Parse() expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Parse() unexpected error: %v", err)
				return
			}

			if msg.Command != tt.wantCmd {
				t.Errorf("Parse() command = %v, want %v", msg.Command, tt.wantCmd)
			}

			if msg.Subject != tt.wantSub {
				t.Errorf("Parse() subject = %v, want %v", msg.Subject, tt.wantSub)
			}
		})
	}
}

func TestParsePub(t *testing.T) {
	tests := []struct {
		name        string
		input       []byte
		wantCmd     byte
		wantSub     string
		wantPayload string
		wantErr     bool
	}{
		{
			name:        "valid publish",
			input:       NewMessageCmd(CmdPub, "foo.bar", []byte("hello")).Encode(),
			wantCmd:     CmdPub,
			wantSub:     "foo.bar",
			wantPayload: "hello",
			wantErr:     false,
		},
		{
			name:        "publish with empty payload",
			input:       NewMessageCmd(CmdPub, "test", nil).Encode(),
			wantCmd:     CmdPub,
			wantSub:     "test",
			wantPayload: "",
			wantErr:     false,
		},
		{
			name:        "publish with large payload",
			input:       NewMessageCmd(CmdPub, "large", make([]byte, 100)).Encode(),
			wantCmd:     CmdPub,
			wantSub:     "large",
			wantPayload: string(make([]byte, 100)),
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, _, err := ParseWithRemaining(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Parse() expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Parse() unexpected error: %v", err)
				return
			}

			if msg.Command != tt.wantCmd {
				t.Errorf("Parse() command = %v, want %v", msg.Command, tt.wantCmd)
			}

			if msg.Subject != tt.wantSub {
				t.Errorf("Parse() subject = %v, want %v", msg.Subject, tt.wantSub)
			}

			if string(msg.Payload) != tt.wantPayload {
				t.Errorf("Parse() payload = %v, want %v", string(msg.Payload), tt.wantPayload)
			}
		})
	}
}

func TestParsePing(t *testing.T) {
	msg := NewMessageCmd(CmdPing, "", nil)
	encoded := msg.Encode()
	parsed, _, err := ParseWithRemaining(encoded)

	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}

	if parsed.Command != CmdPing {
		t.Errorf("Parse() command = %v, want %v", parsed.Command, CmdPing)
	}
}

func TestMessageString(t *testing.T) {
	tests := []struct {
		name    string
		msg     *Message
		wantCmd string
		wantSub string
	}{
		{
			name:    "SUB message",
			msg:     NewMessageCmd(CmdSub, "foo.bar", nil),
			wantCmd: "SUB",
			wantSub: "foo.bar",
		},
		{
			name:    "PUB message",
			msg:     NewMessageCmd(CmdPub, "test", []byte("hello")),
			wantCmd: "PUB",
			wantSub: "test",
		},
		{
			name:    "PING message",
			msg:     NewMessageCmd(CmdPing, "", nil),
			wantCmd: "PING",
			wantSub: "",
		},
		{
			name:    "MSG message",
			msg:     NewMessageCmd(CmdMsg, "foo", []byte("data")),
			wantCmd: "MSG",
			wantSub: "foo",
		},
		{
			name:    "PONG message",
			msg:     NewMessageCmd(CmdPong, "", nil),
			wantCmd: "PONG",
			wantSub: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.msg.String()
			if !stringContains(got, tt.wantCmd) || !stringContains(got, tt.wantSub) {
				t.Errorf("Message.String() = %v, want contains cmd=%v and subject=%v", got, tt.wantCmd, tt.wantSub)
			}
		})
	}
}

func stringContains(s, substr string) bool {
	if substr == "" {
		return true
	}
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestRoundTrip(t *testing.T) {
	// 测试编码和解析的往返
	original := NewMessageCmd(CmdPub, "test.subject", []byte("test payload"))

	encoded := original.Encode()

	deserialized, _, err := ParseWithRemaining(encoded)
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	// 验证
	if deserialized.Command != original.Command {
		t.Errorf("Command mismatch: got %v, want %v", deserialized.Command, original.Command)
	}

	if deserialized.Subject != original.Subject {
		t.Errorf("Subject mismatch: got %v, want %v", deserialized.Subject, original.Subject)
	}

	if string(deserialized.Payload) != string(original.Payload) {
		t.Errorf("Payload mismatch: got %v, want %v", string(deserialized.Payload), string(original.Payload))
	}
}
