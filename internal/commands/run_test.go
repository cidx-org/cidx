package commands

import "testing"

// TestResolveQuiet pins the table of issue #273: which flag decides whether a
// container's output survives a successful run, and the fact that asking for
// the output no longer costs debug logging.
func TestResolveQuiet(t *testing.T) {
	tests := []struct {
		name    string
		quiet   bool
		stream  bool
		verbose bool
		isCI    bool
		want    bool
	}{
		{name: "local runs show their containers", want: false},
		{name: "CI buffers by default", isCI: true, want: true},
		{name: "--stream defeats the CI default", stream: true, isCI: true, want: false},
		{name: "--verbose still defeats it, at the price of debug logs", verbose: true, isCI: true, want: false},
		{name: "--quiet locally is still quiet", quiet: true, want: true},
		{name: "--stream wins over --quiet", quiet: true, stream: true, isCI: true, want: false},
		{name: "--stream outside CI changes nothing", stream: true, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveQuiet(tt.quiet, tt.stream, tt.verbose, tt.isCI); got != tt.want {
				t.Errorf("ResolveQuiet(quiet=%v, stream=%v, verbose=%v, isCI=%v) = %v, want %v",
					tt.quiet, tt.stream, tt.verbose, tt.isCI, got, tt.want)
			}
		})
	}
}

// TestRunDeclaresStream keeps the flag on the command. ci.yml runs
// `cidx run --stream test` for its output; a rename that left the workflow
// behind would put the Test job back to printing nothing (#273).
func TestRunDeclaresStream(t *testing.T) {
	for _, f := range runCommand().Flags {
		for _, name := range f.Names() {
			if name == "stream" {
				return
			}
		}
	}
	t.Fatal("cidx run no longer declares --stream")
}
