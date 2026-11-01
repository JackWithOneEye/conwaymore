package protocol

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecodeSetCellsRejectsMessageShorterThanCount(t *testing.T) {
	// 7282 cells need 65538 bytes, which overflows uint16 arithmetic.
	_, err := DecodeClientMessage([]byte{byte(setCells), 7282 >> 8, 7282 & 0xff, 0, 0})
	assert.Error(t, err)
}
