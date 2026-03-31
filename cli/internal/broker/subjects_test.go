package broker

import "testing"

func TestFlowSubjects(t *testing.T) {
	tests := []struct {
		name string
		fn   func(string, string) string
		want string
	}{
		{
			"request",
			FlowRequestSubject,
			"resystems.renotify.stewart.flow.fl_TEST.request",
		},
		{
			"response",
			FlowResponseSubject,
			"resystems.renotify.stewart.flow.fl_TEST.response",
		},
		{
			"lifecycle",
			FlowLifecycleSubject,
			"resystems.renotify.stewart.flow.fl_TEST.lifecycle",
		},
		{
			"interject",
			FlowInterjectSubject,
			"resystems.renotify.stewart.flow.fl_TEST.interject",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.fn("stewart", "fl_TEST")
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
