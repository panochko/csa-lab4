package golden_test

import (
	"fmt"
	"strconv"
	"strings"
)

// Подмножество YAML, достаточное для эталонов: числа и блочные строки.
// Внешняя библиотека ради этого не нужна.

type field struct {
	key  string
	text *string
	num  *int
}

func marshal(fields []field) string {
	var b strings.Builder
	for _, f := range fields {
		if f.num != nil {
			fmt.Fprintf(&b, "%s: %d\n", f.key, *f.num)
			continue
		}
		s := *f.text
		if s == "" {
			fmt.Fprintf(&b, "%s: \"\"\n", f.key)
			continue
		}
		// |2- без перевода строки в конце, |2+ сохраняет все переводы
		chomp := "-"
		if strings.HasSuffix(s, "\n") {
			chomp, s = "+", strings.TrimSuffix(s, "\n")
		}
		fmt.Fprintf(&b, "%s: |2%s\n", f.key, chomp)
		for _, line := range strings.Split(s, "\n") {
			if line != "" {
				line = "  " + line
			}
			b.WriteString(line + "\n")
		}
	}
	return b.String()
}

func unmarshal(data string, fields []field) error {
	lines := strings.Split(strings.TrimSuffix(data, "\n"), "\n")
	byKey := map[string]field{}
	for _, f := range fields {
		byKey[f.key] = f
	}
	for i := 0; i < len(lines); {
		key, value, ok := strings.Cut(lines[i], ": ")
		f, known := byKey[key]
		if !ok || !known {
			return fmt.Errorf("строка %d: неожиданное содержимое %q", i+1, lines[i])
		}
		i++
		switch {
		case f.num != nil:
			n, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			*f.num = n
		case value == `""`:
			*f.text = ""
		default:
			var block []string
			for ; i < len(lines) && (lines[i] == "" || strings.HasPrefix(lines[i], "  ")); i++ {
				block = append(block, strings.TrimPrefix(lines[i], "  "))
			}
			*f.text = strings.Join(block, "\n")
			if value == "|2+" {
				*f.text += "\n"
			}
		}
	}
	return nil
}
