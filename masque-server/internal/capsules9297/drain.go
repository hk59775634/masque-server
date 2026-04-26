// Package capsules9297 parses RFC 9297 HTTP capsule protocol frames from a byte stream
// (the payload of HTTP/3 DATA frames on a CONNECT stream).
package capsules9297

import (
	"errors"
	"fmt"
	"io"

	"github.com/quic-go/quic-go/quicvarint"
)

// Common errors from Drain.
var (
	ErrNeedMoreData    = errors.New("capsules9297: need more data")
	ErrCapsuleTooLarge = errors.New("capsules9297: capsule length over limit")
	ErrWireLimit       = errors.New("capsules9297: max wire bytes exceeded")
	ErrTrailingBytes   = errors.New("capsules9297: incomplete capsule or trailing bytes at EOF")
)

// Stats summarizes parsed capsules (for logs/metrics).
type Stats struct {
	Capsules  int
	WireBytes int64
	ByType    map[uint64]int
}

// DrainOptions bounds work per CONNECT stream.
type DrainOptions struct {
	MaxWireBytes   int64  // max raw bytes read from r (including varints + payloads)
	MaxCapsuleBody uint64 // max per-capsule payload length

	// PerCapsule, if set, is invoked after each capsule (payload is a copy; errors abort Drain).
	PerCapsule func(typ uint64, payload []byte) error
}

// DefaultMaxCapsuleBody limits single-capsule allocations (1 MiB).
const DefaultMaxCapsuleBody = 1 << 20

// Drain reads from r until io.EOF, parsing consecutive type/length/value capsules.
// Partial frames at EOF return ErrTrailingBytes unless carry is empty.
func Drain(r io.Reader, opt DrainOptions, st *Stats) error {
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
			if opt.PerCapsule != nil {
				pl := make([]byte, len(payload))
				copy(pl, payload)
				if err := opt.PerCapsule(typ, pl); err != nil {
					return err
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

func parseOneCapsule(b []byte, maxBody uint64) (typ uint64, payload []byte, consumed int, err error) {
	if len(b) == 0 {
		return 0, nil, 0, ErrNeedMoreData
	}
	typ, n1, err := quicvarint.Parse(b)
	if err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return 0, nil, 0, ErrNeedMoreData
		}
		return 0, nil, 0, err
	}
	if n1 > len(b) {
		return 0, nil, 0, ErrNeedMoreData
	}
	off := n1
	if off >= len(b) {
		return 0, nil, 0, ErrNeedMoreData
	}
	bodyLen, n2, err := quicvarint.Parse(b[off:])
	if err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return 0, nil, 0, ErrNeedMoreData
		}
		return 0, nil, 0, err
	}
	off += n2
	if bodyLen > maxBody {
		return 0, nil, 0, fmt.Errorf("%w: %d", ErrCapsuleTooLarge, bodyLen)
	}
	if uint64(len(b)-off) < bodyLen {
		return 0, nil, 0, ErrNeedMoreData
	}
	end := off + int(bodyLen)
	return typ, b[off:end], end, nil
}
