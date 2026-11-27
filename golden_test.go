// Package golden_test — интеграционные golden-тесты всей цепочки.
package golden_test

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panochko/csa-lab4/internal/machine"
	"github.com/panochko/csa-lab4/internal/translator"
)

var update = flag.Bool("update", false, "перезаписать эталоны по текущему поведению")

// golden — один тест в одном файле: всё, что подаётся и что получается.
type golden struct {
	source, input                 string
	logHead, logTail              int
	listing, output, summary, log string // машинный код с мнемониками, вывод, итоги, журнал
	noCache                       int    // такты без кэша
}

func (g *golden) fields() []field {
	return []field{
		{key: "in_source", text: &g.source},
		{key: "in_input", text: &g.input},
		{key: "in_log_head", num: &g.logHead},
		{key: "in_log_tail", num: &g.logTail},
		{key: "out_code", text: &g.listing},
		{key: "out_output", text: &g.output},
		{key: "out_summary", text: &g.summary},
		{key: "out_ticks_without_cache", num: &g.noCache},
		{key: "out_log", text: &g.log},
	}
}

type result struct {
	output, summary, log string
	ticks                int
}

func simulate(t *testing.T, image []byte, input string, useCache bool, cfg machine.LogConfig) result {
	t.Helper()
	m, err := machine.New(image, []byte(input), useCache)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	reason, err := machine.Run(m, 5_000_000, &b, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return result{output: string(m.Output()), summary: m.Summary(reason), log: b.String(), ticks: m.Ticks()}
}

func TestGolden(t *testing.T) {
	files, err := filepath.Glob("golden/*.yml")
	if err != nil || len(files) == 0 {
		t.Fatalf("нет golden-тестов: %v", err)
	}
	for _, path := range files {
		name := strings.TrimSuffix(filepath.Base(path), ".yml")
		t.Run(name, func(t *testing.T) { check(t, path, name) })
	}
}

func check(t *testing.T, path, name string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var want golden
	if err := unmarshal(string(raw), want.fields()); err != nil {
		t.Fatal(err)
	}
	got := want
	source, input := readExample(t, name+".lisp"), readExample(t, name+".txt")
	if *update {
		got.source, got.input = source, input
	}
	if got.source != source || got.input != input {
		t.Error("эталон расходится с examples/: запустите go test -update .")
	}
	prog, err := translator.Translate(got.source)
	if err != nil {
		t.Fatal(err)
	}
	got.listing = prog.Listing
	cfg := machine.LogConfig{Head: got.logHead, Tail: got.logTail, Sources: prog.Sources}
	run := simulate(t, prog.Image, got.input, true, cfg)
	got.output, got.summary, got.log = run.output, run.summary, run.log
	got.noCache = simulate(t, prog.Image, got.input, false, machine.LogConfig{}).ticks

	if *update {
		if err := os.WriteFile(path, []byte(marshal(got.fields())), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	compare(t, "out_code", want.listing, got.listing)
	compare(t, "out_output", want.output, got.output)
	compare(t, "out_summary", want.summary, got.summary)
	compare(t, "out_log", want.log, got.log)
	if want.noCache != got.noCache {
		t.Errorf("out_ticks_without_cache: ожидалось %d, получено %d", want.noCache, got.noCache)
	}
}

func readExample(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("examples", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// compare показывает первую различающуюся строку.
func compare(t *testing.T, field, want, got string) {
	t.Helper()
	if want == got {
		return
	}
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := range min(len(w), len(g)) {
		if w[i] != g[i] {
			t.Errorf("%s, строка %d:\n ожидалось: %s\n  получено: %s", field, i+1, w[i], g[i])
			return
		}
	}
	t.Errorf("%s: разное число строк: %d и %d", field, len(w), len(g))
}
