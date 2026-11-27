// Command machine моделирует процессор с точностью до такта.
//
// Использование: machine [флаги] <code.bin> [input.txt]
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/panochko/csa-lab4/internal/machine"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "machine:", err)
		os.Exit(1)
	}
}

func run() (err error) {
	logPath := flag.String("log", "", "файл журнала тактов")
	head := flag.Int("head", -1, "первые N тактов журнала (-1 — все)")
	tail := flag.Int("tail", 0, "последние N тактов журнала")
	limit := flag.Int("limit", 20_000_000, "лимит тактов")
	noCache := flag.Bool("no-cache", false, "отключить кэш (для сравнения)")
	microcode := flag.Bool("microcode", false, "напечатать память микропрограмм и выйти")
	flag.Parse()

	if *microcode {
		fmt.Print(machine.MicrocodeListing())
		return nil
	}
	if flag.NArg() < 1 || flag.NArg() > 2 {
		return errors.New("использование: machine [флаги] <code.bin> [input.txt]")
	}
	image, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		return err
	}
	var input []byte
	if flag.NArg() == 2 {
		if input, err = os.ReadFile(flag.Arg(1)); err != nil {
			return err
		}
	}
	m, err := machine.New(image, input, !*noCache)
	if err != nil {
		return err
	}

	var log io.Writer
	if *logPath != "" {
		file, cerr := os.Create(*logPath)
		if cerr != nil {
			return cerr
		}
		buffered := bufio.NewWriter(file)
		defer func() { err = errors.Join(err, buffered.Flush(), file.Close()) }()
		log = buffered
	}
	cfg := machine.LogConfig{Head: *head, Tail: *tail, Sources: readSources(flag.Arg(0) + ".lst")}
	reason, err := machine.Run(m, *limit, log, cfg)
	fmt.Printf("%s", m.Output())
	if err != nil {
		return err
	}
	fmt.Fprint(os.Stderr, "\n", m.Summary(reason))
	return nil
}

// readSources берёт из листинга соответствие адресов и исходных выражений.
// Без листинга журнал остаётся только с мнемониками.
func readSources(path string) map[uint32]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	sources := map[uint32]string{}
	for _, line := range strings.Split(string(data), "\n") {
		head, source, ok := strings.Cut(line, " ; ")
		addr, _, _ := strings.Cut(head, " ")
		if n, perr := strconv.ParseUint(addr, 16, 32); ok && perr == nil {
			sources[uint32(n)] = source
		}
	}
	return sources
}
