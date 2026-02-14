package protocol

type CommandType byte

const (
	CmdPut CommandType = iota
	CmdGet
	CmdAck
)

type Command struct {
	Type  CommandType
	Key   string
	Value string
}
