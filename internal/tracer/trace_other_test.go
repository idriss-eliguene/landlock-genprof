//go:build !linux

package tracer

import (
	"strings"
	"testing"
)

func TestUnsupportedSourcesSignalAttachmentFailure(t *testing.T) {
	tests := []struct {
		name string
		run  func(func(error)) error
	}{
		{"filesystem", func(done func(error)) error { return TraceFilesystemSourceWithIdentity(nil, Options{}, done, nil) }},
		{"exec", func(done func(error)) error { return TraceExecSourceWithIdentity(nil, Options{}, done, nil) }},
		{"connect", func(done func(error)) error { return TraceConnectSourceWithIdentity(nil, Options{}, done, nil) }},
		{"bind", func(done func(error)) error { return TraceBindSourceWithIdentity(nil, Options{}, done, nil) }},
		{"capabilities", func(done func(error)) error { return TraceCapabilitiesSourceWithIdentity(nil, Options{}, done, nil) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var outcomes []error
			err := tt.run(func(err error) { outcomes = append(outcomes, err) })
			if err == nil || len(outcomes) != 1 || outcomes[0] == nil || !strings.Contains(err.Error(), "not supported on this platform") {
				t.Fatalf("return=%v outcomes=%v", err, outcomes)
			}
		})
	}
}
