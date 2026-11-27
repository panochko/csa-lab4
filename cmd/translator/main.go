// Command translator переводит программу на Lisp в машинный код.
//
// Использование: translator <source.lisp> <target.bin>
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/panochko/csa-lab4/internal/translator"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "translator:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 2 {
		return errors.New("использование: translator <source.lisp> <target.bin>")
	}
	src, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	prog, err := translator.Translate(string(src))
	if err != nil {
		return err
	}
	if err := os.WriteFile(args[1], prog.Image, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(args[1]+".lst", []byte(prog.Listing), 0o600); err != nil {
		return err
	}
	fmt.Printf("%s: %d байт, листинг %s.lst\n", args[1], len(prog.Image), args[1])
	return nil
}
