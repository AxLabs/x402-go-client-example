package permit2

import (
	"testing"

	"github.com/bane-labs-org/x402-go-client-example/internal/x402adapter"
)

func TestRequiresPermit2(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  x402adapter.Requirements
		want bool
	}{
		{
			name: "permit2 extra",
			req: x402adapter.Requirements{
				Extra: map[string]interface{}{"assetTransferMethod": "permit2"},
			},
			want: true,
		},
		{
			name: "eip3009 extra",
			req: x402adapter.Requirements{
				Extra: map[string]interface{}{"assetTransferMethod": "eip3009"},
			},
			want: false,
		},
		{
			name: "no extra",
			req:  x402adapter.Requirements{Scheme: "exact"},
			want: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := RequiresPermit2(tc.req); got != tc.want {
				t.Fatalf("RequiresPermit2() = %v, want %v", got, tc.want)
			}
		})
	}
}
