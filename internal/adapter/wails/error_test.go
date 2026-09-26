package wails

import (
	"errors"
	"fmt"
	"testing"

	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

func TestMarshalError(t *testing.T) {
	revision := int64(4)
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "domain error",
			err: &shared.Error{
				Code:            "revision_conflict",
				Message:         "保存済みデータが更新されています",
				FieldErrors:     map[string]string{"title": "必須です"},
				Retryable:       true,
				CauseID:         "cause-1",
				CurrentRevision: &revision,
			},
			want: `{"code":"revision_conflict","message":"保存済みデータが更新されています","fieldErrors":{"title":"必須です"},"retryable":true,"causeId":"cause-1","currentRevision":4}`,
		},
		{
			name: "wrapped domain error",
			err:  fmt.Errorf("save: %w", &shared.Error{Code: "storage_failed", Message: "保存できませんでした"}),
			want: `{"code":"storage_failed","message":"保存できませんでした","retryable":false}`,
		},
		{
			name: "internal error",
			err:  errors.New("secret path"),
			want: `{"code":"internal","message":"予期しないエラーが発生しました","retryable":false}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(MarshalError(tt.err)); got != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}
