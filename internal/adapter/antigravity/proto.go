package antigravity

// Minimal protobuf wire reader for unlabeled Antigravity gen_metadata blobs.
// Field numbers are reverse-engineered (see docs/provider-usage-semantics.md).

type wireKind int

const (
	wireVarint wireKind = iota
	wireLen
)

type field struct {
	num  uint64
	kind wireKind
	v    uint64
	b    []byte
}

func iterateFields(buf []byte, fn func(field) bool) {
	pos := 0
	for pos < len(buf) {
		tag, n := readVarint(buf[pos:])
		if n == 0 {
			return
		}
		pos += n
		num := tag >> 3
		wt := tag & 7
		switch wt {
		case 0: // varint
			v, n := readVarint(buf[pos:])
			if n == 0 {
				return
			}
			pos += n
			if !fn(field{num: num, kind: wireVarint, v: v}) {
				return
			}
		case 1: // fixed64
			if pos+8 > len(buf) {
				return
			}
			pos += 8
			if !fn(field{num: num, kind: wireVarint, v: 0}) {
				return
			}
		case 2: // length-delimited
			l, n := readVarint(buf[pos:])
			if n == 0 {
				return
			}
			pos += n
			end := pos + int(l)
			if end > len(buf) || int(l) < 0 {
				return
			}
			if !fn(field{num: num, kind: wireLen, b: buf[pos:end]}) {
				return
			}
			pos = end
		case 5: // fixed32
			if pos+4 > len(buf) {
				return
			}
			pos += 4
			if !fn(field{num: num, kind: wireVarint, v: 0}) {
				return
			}
		default:
			return
		}
	}
}

func readVarint(buf []byte) (uint64, int) {
	var result uint64
	var shift uint
	for i, b := range buf {
		if i > 9 {
			return 0, 0
		}
		result |= uint64(b&0x7f) << shift
		if b&0x80 == 0 {
			return result, i + 1
		}
		shift += 7
	}
	return 0, 0
}

func messageField(buf []byte, num uint64) []byte {
	var out []byte
	iterateFields(buf, func(f field) bool {
		if f.num == num && f.kind == wireLen {
			out = f.b
			return false
		}
		return true
	})
	return out
}

func varintField(buf []byte, num uint64) (uint64, bool) {
	var v uint64
	var ok bool
	iterateFields(buf, func(f field) bool {
		if f.num == num && f.kind == wireVarint {
			v = f.v
			ok = true
			return false
		}
		return true
	})
	return v, ok
}

func stringField(buf []byte, num uint64) string {
	b := messageField(buf, num)
	if b == nil {
		return ""
	}
	return string(b)
}

func protoTimestampMS(ts []byte) int64 {
	sec, ok := varintField(ts, 1)
	if !ok {
		return 0
	}
	nanos, _ := varintField(ts, 2)
	return int64(sec)*1000 + int64(nanos)/1_000_000
}
