package machine

import (
	"errors"
	"fmt"
	"io"
)

// LogConfig задаёт, какая часть журнала сохраняется.
type LogConfig struct {
	Head, Tail int // первые и последние N тактов; Head < 0 — весь журнал
	Sources    map[uint32]string
}

// Run моделирует работу до halt, конца ввода, ошибки или лимита тактов.
func Run(m *Machine, limit int, log io.Writer, cfg LogConfig) (string, error) {
	var tail []string
	skipped, written := 0, 0
	emit := func(line string) error {
		if cfg.Head < 0 || written < cfg.Head {
			written++
			_, err := fmt.Fprintln(log, line)
			return err
		}
		tail = append(tail, line)
		if len(tail) > cfg.Tail {
			tail = tail[1:]
			skipped++
		}
		return nil
	}
	flush := func() error {
		if skipped > 0 {
			if _, err := fmt.Fprintf(log, "... пропущено тактов: %d ...\n", skipped); err != nil {
				return err
			}
		}
		for _, line := range tail {
			if _, err := fmt.Fprintln(log, line); err != nil {
				return err
			}
		}
		return nil
	}
	for m.Ticks() < limit {
		executed := m.MicroPC()
		err := m.Tick()
		if log != nil {
			if werr := emit(m.State(executed, cfg.Sources)); werr != nil {
				return "", werr
			}
		}
		if err == nil {
			continue
		}
		if log != nil {
			if werr := flush(); werr != nil {
				return "", werr
			}
		}
		if errors.Is(err, ErrHalted) || errors.Is(err, ErrInputEmpty) {
			return err.Error(), nil
		}
		return "", fmt.Errorf("такт %d: %w", m.Ticks(), err)
	}
	return "", fmt.Errorf("превышен лимит %d тактов", limit)
}

// Summary — итоговые показатели моделирования.
func (m *Machine) Summary(reason string) string {
	s := m.Stats()
	return fmt.Sprintf("stop: %s\nticks: %d\ninstructions: %d\ncache read hits: %d\ncache read misses: %d\nmemory writes: %d\nstall ticks: %d\n",
		reason, m.Ticks(), m.Instructions(), s.ReadHits, s.ReadMisses, s.Writes, s.StallTicks)
}
