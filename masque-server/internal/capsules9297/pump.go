package capsules9297

import (
	"fmt"
	"io"
)

// Pump reads RFC 9297 capsules from r; after each capsule, fn may return bytes to write to w
// (typically further capsules on the same CONNECT stream). Same limits and stats as Drain.
func Pump(r io.Reader, w io.Writer, opt DrainOptions, st *Stats, fn func(typ uint64, payload []byte) ([][]byte, error)) error {
	if fn == nil {
		return fmt.Errorf("capsules9297: Pump requires fn")
	}
	if st.ByType == nil {
		st.ByType = make(map[uint64]int)
	}
	if opt.MaxWireBytes <= 0 {
		opt.MaxWireBytes = 1 << 30
	}
	if opt.MaxCapsuleBody == 0 {
		opt.MaxCapsuleBody = DefaultMaxCapsuleBody
	}

	buf := make([]byte, 32<<10)
	var carry []byte
	for {
		if st.WireBytes >= opt.MaxWireBytes {
			return fmt.Errorf("%w: read %d bytes", ErrWireLimit, st.WireBytes)
		}
		room := int64(len(buf))
		left := opt.MaxWireBytes - st.WireBytes
		if int64(len(buf)) > left {
			room = left
		}
		if room <= 0 {
			return fmt.Errorf("%w", ErrWireLimit)
		}
		n, err := r.Read(buf[:room])
		st.WireBytes += int64(n)
		carry = append(carry, buf[:n]...)

		for {
			typ, payload, consumed, perr := parseOneCapsule(carry, opt.MaxCapsuleBody)
			if perr == ErrNeedMoreData {
				break
			}
			if perr != nil {
				return perr
			}
			st.Capsules++
			st.ByType[typ]++
			pl := make([]byte, len(payload))
			copy(pl, payload)
			replies, ferr := fn(typ, pl)
			if ferr != nil {
				return ferr
			}
			for _, chunk := range replies {
				if len(chunk) == 0 {
					continue
				}
				if _, werr := w.Write(chunk); werr != nil {
					return werr
				}
			}
			carry = carry[consumed:]
		}

		if err == io.EOF {
			if len(carry) > 0 {
				return fmt.Errorf("%w: %d trailing bytes", ErrTrailingBytes, len(carry))
			}
			return nil
		}
		if err != nil {
			return err
		}
	}
}
