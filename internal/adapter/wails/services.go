package wails

type (
	StartupService      struct{}
	ProjectService      struct{}
	PreparationService  struct{}
	ExecutionService    struct{}
	ProcedureService    struct{}
	AgentControlService struct{}
)

func NonNilSlice[T any](items []T) []T {
	if items == nil {
		return []T{}
	}

	return items
}
