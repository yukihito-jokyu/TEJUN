package project

import "github.com/yukihito-jokyu/TEJUN/internal/domain/shared"

type Status string

const (
	Preparing        Status = "preparing"
	AIRunning        Status = "ai_running"
	HumanWaiting     Status = "human_waiting"
	ProcedureEditing Status = "procedure_editing"
	Completed        Status = "completed"
	Error            Status = "error"
	Archived         Status = "archived"
)

func ValidStatus(status Status) bool {
	switch status {
	case Preparing, AIRunning, HumanWaiting, ProcedureEditing, Completed, Error, Archived:
		return true
	default:
		return false
	}
}

func CanRevise(status Status) error {
	if status != Completed {
		return &shared.Error{Code: "invalid_project_state", Message: "完成したプロジェクトだけ改訂できます"}
	}

	return nil
}

func CanArchive(status Status, activeJob bool) error {
	if activeJob {
		return &shared.Error{Code: "active_job", Message: "処理中のプロジェクトはアーカイブできません"}
	}

	if status == Archived {
		return &shared.Error{Code: "invalid_project_state", Message: "既にアーカイブされています"}
	}

	return nil
}

func ResumeRoute(id string, stage string) string {
	switch stage {
	case "execution":
		return "#/projects/" + id + "/check"
	case "procedure", "completed":
		return "#/projects/" + id + "/procedure"
	default:
		return "#/projects/" + id + "/prepare"
	}
}
