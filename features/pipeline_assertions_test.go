package features

import (
	"testing"

	"github.com/cucumber/godog"
	messages "github.com/cucumber/messages/go/v21"
)

// An order assertion must reject a nonempty but wrong execution trace.
func TestPipelineOrderAssertionsRejectIncorrectTraces(t *testing.T) {
	row := func(values ...string) *messages.PickleTableRow {
		r := &messages.PickleTableRow{}
		for _, value := range values {
			r.Cells = append(r.Cells, &messages.PickleTableCell{Value: value})
		}
		return r
	}
	for _, trace := range [][]string{
		nil, {"code"}, {"security", "code", "test"},
		{"code", "security", "test", "build"}, {"code", "security", "security", "test"},
	} {
		tc := NewTestContext()
		tc.ExecutedPhases = trace
		for name, check := range map[string]func() error{
			"string": func() error { return tc.phasesShouldExecuteInOrder("code, security, test") },
			"numbered": func() error {
				return tc.phasesShouldExecuteInExactOrder(&godog.Table{Rows: []*messages.PickleTableRow{
					row("order", "phase"), row("1", "code"), row("2", "security"), row("3", "test"),
				}})
			},
			"table": func() error {
				return tc.phasesShouldExecuteInOrderTable(&godog.Table{Rows: []*messages.PickleTableRow{
					row("code"), row("security"), row("test"),
				}})
			},
			"list": func() error { return tc.shouldExecutePhasesList("code, security, test") },
		} {
			if err := check(); err == nil {
				t.Errorf("%s accepted incorrect trace %v", name, trace)
			}
		}
	}
}
