// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// keyString encodes a relation key tuple as a comparable string. Each
// component is type-tagged so 1 and "1" differ, and components are separated
// by 0x1f so ("a","b") and ("ab","") differ. Values are normalized so that a
// key built from a struct field (int, named string) matches the same key read
// back from the driver (int64, []byte, driver.Valuer output).
func keyString(vals []any) string {
	var sb strings.Builder
	for i, v := range vals {
		if i > 0 {
			sb.WriteByte(0x1f)
		}
		writeKeyPart(&sb, v)
	}
	return sb.String()
}

func writeKeyPart(sb *strings.Builder, v any) {
	switch x := v.(type) {
	case nil:
		sb.WriteString("n:")
	case string:
		sb.WriteString("s:")
		sb.WriteString(x)
	case []byte:
		sb.WriteString("s:")
		sb.Write(x)
	case int:
		sb.WriteString("i:" + strconv.FormatInt(int64(x), 10))
	case int8:
		sb.WriteString("i:" + strconv.FormatInt(int64(x), 10))
	case int16:
		sb.WriteString("i:" + strconv.FormatInt(int64(x), 10))
	case int32:
		sb.WriteString("i:" + strconv.FormatInt(int64(x), 10))
	case int64:
		sb.WriteString("i:" + strconv.FormatInt(x, 10))
	case uint:
		sb.WriteString("u:" + strconv.FormatUint(uint64(x), 10))
	case uint8:
		sb.WriteString("u:" + strconv.FormatUint(uint64(x), 10))
	case uint16:
		sb.WriteString("u:" + strconv.FormatUint(uint64(x), 10))
	case uint32:
		sb.WriteString("u:" + strconv.FormatUint(uint64(x), 10))
	case uint64:
		sb.WriteString("u:" + strconv.FormatUint(x, 10))
	case bool:
		sb.WriteString("b:" + strconv.FormatBool(x))
	case float32:
		sb.WriteString("f:" + strconv.FormatFloat(float64(x), 'g', -1, 64))
	case float64:
		sb.WriteString("f:" + strconv.FormatFloat(x, 'g', -1, 64))
	case time.Time:
		sb.WriteString("t:" + x.UTC().Format(time.RFC3339Nano))
	case driver.Valuer:
		vv, err := x.Value()
		if err != nil {
			sb.WriteString("e:" + err.Error())
			return
		}
		writeKeyPart(sb, vv)
	default:
		fmt.Fprintf(sb, "x:%T:%v", x, x)
	}
}
