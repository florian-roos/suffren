package protocol

type CommandType byte

const (
	CmdPut CommandType = iota
	CmdGet
	CmdAck
	CmdNack
)

type Command struct {
	Type  CommandType
	Key   string
	Value string
}
