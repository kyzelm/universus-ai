package sim

import (
	"fmt"
	"reflect"
	"strings"
)

// Diagnostic only.

// Dump renders the whole state as one field per line, in declaration order.
func (s *GameState) Dump() string {
	var b strings.Builder
	dumpValue(&b, "", reflect.ValueOf(*s))
	return b.String()
}

func dumpValue(b *strings.Builder, path string, v reflect.Value) {
	switch v.Kind() {
	case reflect.Struct:
		t := v.Type()
		for i := range t.NumField() {
			name := t.Field(i).Name
			if path != "" {
				name = path + "." + name
			}
			dumpValue(b, name, v.Field(i))
		}

	case reflect.Array:
		// Zero runs are collapsed: the input history is 32 entries per player
		// and is mostly zero, and 60 lines of "0" per dump buries the two that
		// matter.
		n := v.Len()
		for i := 0; i < n; {
			if !isZero(v.Index(i)) {
				dumpValue(b, fmt.Sprintf("%s[%d]", path, i), v.Index(i))
				i++
				continue
			}
			j := i
			for j < n && isZero(v.Index(j)) {
				j++
			}
			if j-i == 1 {
				dumpValue(b, fmt.Sprintf("%s[%d]", path, i), v.Index(i))
			} else {
				fmt.Fprintf(b, "%s[%d:%d] = zero\n", path, i, j)
			}
			i = j
		}

	default:
		fmt.Fprintf(b, "%s = %v\n", path, v.Interface())
	}
}

func isZero(v reflect.Value) bool {
	return v.Kind() != reflect.Struct && v.Kind() != reflect.Array && v.IsZero()
}
