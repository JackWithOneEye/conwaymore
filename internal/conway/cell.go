package conway

type Cell interface {
	Values() (x, y uint16, colour uint32, age uint16)
}

type aliveCell struct {
	x      uint16
	y      uint16
	colour uint32
	age    uint16
}

func (ac *aliveCell) Values() (x, y uint16, colour uint32, age uint16) {
	return ac.x, ac.y, ac.colour, ac.age
}

func (ac *aliveCell) copy(other *aliveCell) {
	ac.x = other.x
	ac.y = other.y
	ac.colour = other.colour
	ac.age = other.age
}

func (ac *aliveCell) set(x, y uint16, colour uint32, age uint16) {
	ac.x = x
	ac.y = y
	ac.colour = colour
	ac.age = age
}

type cellCandidate struct {
	x               uint16
	y               uint16
	colour          uint32
	age             uint16
	alive           bool
	aliveNeighbours []aliveCell
	count           uint8
}

func (cn *cellCandidate) addNeighbour(neighbour *aliveCell) {
	if cn.count < 3 {
		cn.aliveNeighbours[cn.count].copy(neighbour)
	}
	cn.count += 1
}

func (cn *cellCandidate) create() {
	n0 := cn.aliveNeighbours[0]
	n1 := cn.aliveNeighbours[1]
	n2 := cn.aliveNeighbours[2]

	// sort clockwise
	if n0.x < n1.x || n0.y < n1.y {
		if n2.x < n0.x || n2.y < n0.y {
			n0 = cn.aliveNeighbours[2]
			n1 = cn.aliveNeighbours[0]
			n2 = cn.aliveNeighbours[1]
		} else if n2.x < n1.x || n2.y < n1.y {
			n1 = cn.aliveNeighbours[2]
			n2 = cn.aliveNeighbours[1]
		}
	} else {
		if n0.x < n2.x || n0.y < n2.y {
			n0 = cn.aliveNeighbours[1]
			n1 = cn.aliveNeighbours[0]
			n2 = cn.aliveNeighbours[2]
		} else if n1.x < n2.x || n1.y < n2.y {
			n0 = cn.aliveNeighbours[1]
			n1 = cn.aliveNeighbours[2]
			n2 = cn.aliveNeighbours[0]
		} else {
			n0 = cn.aliveNeighbours[2]
			n2 = cn.aliveNeighbours[0]
		}
	}
	cn.colour = (n0.colour & 0xff0000) | (n1.colour & 0x00ff00) | (n2.colour & 0x0000ff)
}

func (cn *cellCandidate) markAsAlive(cell *aliveCell) {
	cn.colour = cell.colour
	cn.age = cell.age
	cn.alive = true
}
