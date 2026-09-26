//go:build !darwin && !linux

package export

import (
	"os"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

func fileID(os.FileInfo) (uint64, uint64) { return 0, 0 }

func exchangeChecked(*os.Root, string, string, application.OverwriteIdentity) error {
	return &shared.Error{Code: "export_unsupported", Message: "この環境では安全な上書きを利用できません"}
}
