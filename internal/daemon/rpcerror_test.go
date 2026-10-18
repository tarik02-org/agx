package daemon

import (
	"errors"
	"testing"

	"connectrpc.com/connect"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
)

func TestErrorReasons(t *testing.T) {
	for _, test := range []struct {
		err  error
		code connect.Code
		want v1.ErrorDetail_Reason
	}{
		{rpcError(connect.CodeNotFound, "x"), connect.CodeNotFound, v1.ErrorDetail_REASON_NOT_FOUND},
		{rpcError(connect.CodeAborted, "x"), connect.CodeAborted, v1.ErrorDetail_REASON_CONFLICT},
		// A code without a documented meaning is a defect.
		{rpcError(connect.CodeUnknown, "x"), connect.CodeUnknown, v1.ErrorDetail_REASON_INTERNAL},
		{configError(errors.New("x")), connect.CodeFailedPrecondition, v1.ErrorDetail_REASON_CONFIG_INVALID},
		{refusal(), connect.CodeUnavailable, v1.ErrorDetail_REASON_UNAVAILABLE},
	} {
		var rpc *connect.Error
		if !errors.As(test.err, &rpc) || rpc.Code() != test.code {
			t.Fatalf("%v: code %v, want %v", test.err, connect.CodeOf(test.err), test.code)
		}
		reasons := []v1.ErrorDetail_Reason{}
		for _, d := range rpc.Details() {
			value, err := d.Value()
			if err != nil {
				t.Fatal(err)
			}
			if detail, ok := value.(*v1.ErrorDetail); ok {
				reasons = append(reasons, detail.Reason)
			}
		}
		if len(reasons) != 1 || reasons[0] != test.want {
			t.Errorf("%v: reasons %v, want %v", test.err, reasons, test.want)
		}
	}
}
