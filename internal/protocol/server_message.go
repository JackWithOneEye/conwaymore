package protocol

import (
	"encoding/binary"
	"errors"
)

// StateUpdateMarker distinguishes a StateUpdate from an Output frame: an Output
// starts with the playing flag (0 or 1), a StateUpdate starts with 0xff.
const StateUpdateMarker = 0xff

const stateUpdateLen = 4

// StateUpdate is a lightweight server -> client message carrying only the play
// state and speed. It is broadcast instead of a full Output frame when no cell
// changed (e.g. on a speed change), saving hundreds of KB per message.
type StateUpdate struct {
	Playing bool
	Speed   uint16
}

func (s *StateUpdate) Encode() []byte {
	b := make([]byte, stateUpdateLen)
	b[0] = StateUpdateMarker
	if s.Playing {
		b[1] = 1
	}
	binary.BigEndian.PutUint16(b[2:], s.Speed)
	return b
}

func DecodeStateUpdate(b []byte) (*StateUpdate, error) {
	if !IsStateUpdate(b) {
		return nil, errors.New("[StateUpdate] not a state update")
	}
	if len(b) != stateUpdateLen {
		return nil, errors.New("[StateUpdate] invalid length")
	}
	return &StateUpdate{
		Playing: b[1] == 1,
		Speed:   binary.BigEndian.Uint16(b[2:]),
	}, nil
}

// IsStateUpdate reports whether b is a StateUpdate rather than an Output frame.
func IsStateUpdate(b []byte) bool {
	return len(b) > 0 && b[0] == StateUpdateMarker
}

// ApplyStateUpdate returns a copy of an encoded Output frame with the playing
// flag and speed in its header replaced by the state update's values, leaving
// the (potentially large) cell section untouched. It lets a pending full frame
// absorb a state update so neither is lost. If frame is itself a state update
// or is malformed, the state update is returned (it fully supersedes both).
func ApplyStateUpdate(frame, su []byte) []byte {
	if !IsStateUpdate(su) || len(su) != stateUpdateLen {
		return frame
	}
	if IsStateUpdate(frame) || len(frame) < cellsOffset {
		return su
	}
	patched := make([]byte, len(frame))
	copy(patched, frame)
	patched[0] = su[1]
	patched[1] = su[2]
	patched[2] = su[3]
	return patched
}
