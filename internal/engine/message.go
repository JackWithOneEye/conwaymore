package engine

const ( // byte 0
	command  uint = 1 // byte 1
	setSpeed uint = 2 // byte 2-3 (1 ms to 65536 ms)
	setCells uint = 4 // byte 4: #cells; byte 5-: cells
)

type messageIndex = uint

const (
	msgType    messageIndex = 0
	cmd        messageIndex = 1
	speed      messageIndex = 2
	cellsCount messageIndex = 4
	cells      messageIndex = 5
)

type commandType = uint

const (
	next commandType = iota
	play
	pause
	clear
	randomise
)
