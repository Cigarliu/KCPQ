package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	MaxSubjectLen = 4 * 1024
	MaxBodyLen    = 8 * 1024 * 1024
)

// 协议命令类型
const (
	CmdSub   = byte(0x01)
	CmdUnsub = byte(0x02)
	CmdPub   = byte(0x03)
	CmdMsg   = byte(0x04)
	CmdPing  = byte(0x05)
	CmdPong  = byte(0x06)
	CmdOk    = byte(0x07)
	CmdErr   = byte(0x08)
)

// Message 表示一条协议消息
type Message struct {
	Command byte
	Subject string
	Payload []byte
}

// NewMessage 创建新消息
func NewMessageCmd(command byte, subject string, payload []byte) *Message {
	return &Message{
		Command: command,
		Subject: subject,
		Payload: payload,
	}
}

// NewMessage 创建新消息（兼容旧接口）
func NewMessage(commandStr, subject string, payload []byte) *Message {
	cmd := cmdStringToByte(commandStr)
	return NewMessageCmd(cmd, subject, payload)
}

func cmdStringToByte(cmdStr string) byte {
	switch cmdStr {
	case "SUB":
		return CmdSub
	case "UNSUB":
		return CmdUnsub
	case "PUB":
		return CmdPub
	case "MSG":
		return CmdMsg
	case "PING":
		return CmdPing
	case "PONG":
		return CmdPong
	case "OK":
		return CmdOk
	case "ERR":
		return CmdErr
	default:
		return 0
	}
}

func cmdByteToString(cmd byte) string {
	switch cmd {
	case CmdSub:
		return "SUB"
	case CmdUnsub:
		return "UNSUB"
	case CmdPub:
		return "PUB"
	case CmdMsg:
		return "MSG"
	case CmdPing:
		return "PING"
	case CmdPong:
		return "PONG"
	case CmdOk:
		return "OK"
	case CmdErr:
		return "ERR"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", cmd)
	}
}

// Encode 将消息编码为二进制格式
// 格式: [4字节消息体长度][1字节命令][2字节subject长度][subject][payload]
// 其中"消息体长度"指后续数据大小(不含4字节长度前缀本身)
func (m *Message) Encode() []byte {
	subjectBytes := []byte(m.Subject)
	subjectLen := len(subjectBytes)
	payloadLen := len(m.Payload)

	// 计算消息体大小: 1(命令) + 2(subject长度) + subject长度 + payload长度
	bodyLen := 1 + 2 + subjectLen + payloadLen
	totalLen := 4 + bodyLen

	// 直接分配并写入，减少 bytes.Buffer 带来的额外开销
	out := make([]byte, totalLen)
	// 写入长度
	binary.BigEndian.PutUint32(out[0:4], uint32(bodyLen))
	// 写入命令
	out[4] = m.Command
	// 写入 subject 长度
	binary.BigEndian.PutUint16(out[5:7], uint16(subjectLen))
	// 写入 subject
	copy(out[7:7+subjectLen], subjectBytes)
	// 写入 payload
	copy(out[7+subjectLen:], m.Payload)

	return out
}

// String 兼容旧接口，返回调试信息
func (m *Message) String() string {
	return fmt.Sprintf("[%s] %s (%d bytes)", cmdByteToString(m.Command), m.Subject, len(m.Payload))
}

// Parse 从字节数组解析消息
func Parse(data []byte) (*Message, error) {
	msg, _, err := ParseWithRemaining(data)
	return msg, err
}

// ParseWithRemaining 从字节数组解析消息，并返回剩余未解析的数据
func ParseWithRemaining(data []byte) (*Message, []byte, error) {
	if len(data) < 7 { // 最小长度: 4(length) + 1(cmd) + 2(subject_len)
		return nil, data, errors.New("data too short")
	}

	// 读取消息体长度
	bodyLen := int(binary.BigEndian.Uint32(data[0:4]))
	if bodyLen <= 0 || bodyLen > MaxBodyLen {
		return nil, data, fmt.Errorf("invalid body length: %d", bodyLen)
	}
	totalPacketLen := bodyLen + 4

	// 检查数据是否完整
	if len(data) < totalPacketLen {
		// 数据不完整，返回原数据等待更多数据
		return nil, data, fmt.Errorf("incomplete message: need %d bytes, got %d",
			totalPacketLen, len(data))
	}

	// 提取消息数据
	msgData := data[4:totalPacketLen]
	offset := 0

	// 读取命令
	cmd := msgData[offset]
	offset += 1

	// 读取subject长度
	subjectLen := int(binary.BigEndian.Uint16(msgData[offset : offset+2]))
	offset += 2

	// 检查subject长度是否合法
	if subjectLen < 0 || subjectLen > MaxSubjectLen || offset+subjectLen > len(msgData) {
		return nil, data, fmt.Errorf("invalid subject length: %d, available: %d",
			subjectLen, len(msgData)-offset)
	}

	// 读取subject
	subject := string(msgData[offset : offset+subjectLen])
	offset += subjectLen

	// 读取payload
	payloadLen := len(msgData) - offset
	payload := make([]byte, payloadLen)
	copy(payload, msgData[offset:])

	msg := &Message{
		Command: cmd,
		Subject: subject,
		Payload: payload,
	}

	// 返回消息和剩余数据
	remaining := data[totalPacketLen:]
	return msg, remaining, nil
}

// ParseFromReader 从io.Reader读取并解析消息（用于流式读取）
func ParseFromReader(reader io.Reader) (*Message, error) {
	// 读取长度前缀
	var lengthBuf [4]byte
	if _, err := io.ReadFull(reader, lengthBuf[:]); err != nil {
		return nil, fmt.Errorf("failed to read length: %w", err)
	}

	bodyLen := int(binary.BigEndian.Uint32(lengthBuf[:]))
	if bodyLen <= 0 || bodyLen > MaxBodyLen {
		return nil, fmt.Errorf("invalid body length: %d", bodyLen)
	}

	// 读取消息体
	msgData := make([]byte, bodyLen)
	if _, err := io.ReadFull(reader, msgData); err != nil {
		return nil, fmt.Errorf("failed to read message data: %w", err)
	}

	// 解析消息体
	offset := 0
	if offset >= len(msgData) {
		return nil, errors.New("empty message body")
	}

	// 读取命令
	cmd := msgData[offset]
	offset += 1

	// 读取subject长度
	if offset+2 > len(msgData) {
		return nil, errors.New("message too short for subject length")
	}
	subjectLen := int(binary.BigEndian.Uint16(msgData[offset : offset+2]))
	offset += 2

	// 检查subject长度是否合法
	if subjectLen < 0 || subjectLen > MaxSubjectLen || offset+subjectLen > len(msgData) {
		return nil, fmt.Errorf("invalid subject length: %d, available: %d",
			subjectLen, len(msgData)-offset)
	}

	// 读取subject
	subject := string(msgData[offset : offset+subjectLen])
	offset += subjectLen

	// 读取payload
	// 这里的 payload 直接引用 msgData 的切片，避免拷贝
	// 注意：msgData 的生命周期需要由调用者管理，或者在这里拷贝
	// 在本场景中，msgData 是每次 ReadFull 分配的，引用它是安全的，只要 msg 不被长期持有
	payload := msgData[offset:]

	msg := &Message{
		Command: cmd,
		Subject: subject,
		Payload: payload,
	}

	return msg, nil
}
