package history

import "fmt"

const maxInt64Count = uint64(1<<63 - 1)

func checkedUint32Length(length int) (uint32, error) {
	if length < 0 || uint64(length) > uint64(^uint32(0)) {
		return 0, fmt.Errorf("canonical field length exceeds uint32")
	}
	// The range check above proves this conversion is lossless. gosec cannot
	// carry that local proof through the conversion itself.
	return uint32(length), nil // #nosec G115 -- checked against MaxUint32
}

func checkedInt64Count(value uint64) (int64, error) {
	if value > maxInt64Count {
		return 0, fmt.Errorf("count exceeds int64")
	}
	return int64(value), nil // #nosec G115 -- checked against MaxInt64
}
