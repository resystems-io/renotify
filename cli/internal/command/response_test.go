package command

import "testing"

func TestIsErrorResponse(t *testing.T) {
	tests := []struct {
		name string
		data string
		want bool
	}{
		{
			name: "error response with code",
			data: `{"code":"timeout","message":"ask timed out"}`,
			want: true,
		},
		{
			name: "notification response without code",
			data: `{"notification_id":"ntf_01","accepted":true}`,
			want: false,
		},
		{
			name: "empty code field",
			data: `{"code":""}`,
			want: false,
		},
		{
			name: "malformed JSON",
			data: `not json`,
			want: false,
		},
		{
			name: "empty input",
			data: ``,
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isErrorResponse([]byte(tc.data))
			if got != tc.want {
				t.Errorf("isErrorResponse(%s) = %v, want %v",
					tc.data, got, tc.want)
			}
		})
	}
}
