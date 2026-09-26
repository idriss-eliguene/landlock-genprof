package proposal

import "testing"

func TestCheckedUint32LengthBoundaries(t *testing.T) {
	max := int(^uint32(0))
	for _, value := range []int{0, 1, max} {
		if got, err := checkedUint32Length(value); err != nil || got != uint32(value) {
			t.Fatalf("checkedUint32Length(%d) = %d, %v", value, got, err)
		}
	}
	if _, err := checkedUint32Length(max + 1); err == nil || err.Error() != "canonical field length exceeds uint32" {
		t.Fatalf("checkedUint32Length accepted or misreported the first out-of-range value: %v", err)
	}
}
