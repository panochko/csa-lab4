package machine

import (
	"encoding/binary"
	"fmt"

	"github.com/panochko/csa-lab4/internal/isa"
)

// Параметры кэша прямого отображения.
const (
	cacheLines  = 64
	lineWords   = 4
	memoryDelay = 10 // тактов на слово
)

type cacheLine struct {
	valid bool
	tag   uint32
	data  [lineWords]uint32
}

// CacheStats — счётчики обращений.
type CacheStats struct {
	ReadHits, ReadMisses, Writes, StallTicks int
}

// cache — контроллер кэша вместе с однопортовой памятью.
// Политика записи: write-through без аллокации.
type cache struct {
	enabled bool
	mem     []byte
	lines   [cacheLines]cacheLine
	wait    int // тактов, уже потраченных на текущее обращение
	filling bool
	stats   CacheStats
}

// split делит адрес на тег, номер строки и номер слова в строке.
func split(addr uint32) (uint32, uint32, uint32) {
	return addr >> 10, addr >> 4 & 0x3F, addr >> 2 & 0x3
}

func (c *cache) check(addr uint32) error {
	if addr%isa.WordSize != 0 || addr > isa.MemorySize-isa.WordSize {
		return fmt.Errorf("недопустимый адрес %#x", addr)
	}
	return nil
}

func (c *cache) load(addr uint32) uint32 { return binary.LittleEndian.Uint32(c.mem[addr:]) }

// read выполняет один такт чтения. ready=false означает, что процессор ждёт.
// Возвращает значение, признак готовности и описание события.
func (c *cache) read(addr uint32) (uint32, bool, string) {
	if !c.enabled {
		c.wait++
		if c.wait < memoryDelay {
			c.stats.StallTicks++
			return 0, false, fmt.Sprintf("MEM read %d/%d", c.wait, memoryDelay)
		}
		c.wait = 0
		return c.load(addr), true, "MEM read done"
	}
	tag, index, word := split(addr)
	line := &c.lines[index]
	if !c.filling && line.valid && line.tag == tag {
		c.stats.ReadHits++
		return line.data[word], true, fmt.Sprintf("HIT line %d", index)
	}
	if !c.filling {
		c.filling, line.valid, c.wait = true, false, 0
		c.stats.ReadMisses++
	}
	c.wait++
	c.stats.StallTicks++
	n := (c.wait - 1) / memoryDelay // номер загружаемого слова
	event := fmt.Sprintf("MISS line %d: word %d, tick %d/%d", index, n, (c.wait-1)%memoryDelay+1, memoryDelay)
	if c.wait%memoryDelay == 0 {
		base := addr &^ uint32(lineWords*isa.WordSize-1)
		line.data[n] = c.load(base + uint32(n)*isa.WordSize)
	}
	if c.wait == lineWords*memoryDelay {
		line.valid, line.tag, c.filling, c.wait = true, tag, false, 0
		// следующее обращение найдёт строку, но это то же самое чтение
		c.stats.ReadHits--
	}
	return 0, false, event
}

// write выполняет один такт записи.
func (c *cache) write(addr, value uint32) (bool, string) {
	c.wait++
	if c.wait < memoryDelay {
		c.stats.StallTicks++
		return false, fmt.Sprintf("WRITE-THROUGH %d/%d", c.wait, memoryDelay)
	}
	c.wait = 0
	c.stats.Writes++
	binary.LittleEndian.PutUint32(c.mem[addr:], value)
	tag, index, word := split(addr)
	if line := &c.lines[index]; c.enabled && line.valid && line.tag == tag {
		line.data[word] = value
		return true, fmt.Sprintf("WRITE done, line %d updated", index)
	}
	return true, "WRITE done"
}
