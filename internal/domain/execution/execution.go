package execution

import "time"

type Check struct {
	ID                          string
	Sequence                    int
	AIRequired, HumanRequired   bool
	Title, Instruction          string
	ExpectedResult              string
	AIStatus, HumanStatus       string
	AICheckedAt, HumanCheckedAt *time.Time
	AIFailureSummary            string
	HumanEvidenceRequirement    string
	Evidence                    []Evidence
}

type Evidence struct {
	ID, CheckID, Actor, Kind, Text, DisplayName, BlobHash, Status string
	MimeType                                                      string
	Size                                                          int64
	CreatedAt                                                     time.Time
}

type Permission struct {
	ID, SessionID, RunID, ToolCallID, Title, Status string
	Options                                         []PermissionOption
	RequestedAt                                     time.Time
	ExpiresAt                                       *time.Time
}

type PermissionOption struct {
	ID   string `json:"optionId"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type Turn struct {
	ID, Role, Text, Status string
	CreatedAt              time.Time
}

type Snapshot struct {
	ProjectID, ExecutionID, Status, SessionID, ActiveRunID string
	Revision, ChangeSequence                               int64
	StartedAt                                              time.Time
	CompletedAt                                            *time.Time
	Checks                                                 []Check
	Permissions                                            []Permission
	Conversation                                           []Turn
	CanGenerate                                            bool
	BlockingReasons                                        []string
}

func (s Snapshot) Readiness() (bool, []string) {
	reasons := []string{}
	if s.Status != "active" {
		reasons = append(reasons, "execution_not_active")
	}

	if len(s.Checks) == 0 {
		reasons = append(reasons, "checks_empty")
	}

	for _, check := range s.Checks {
		if (check.AIRequired && check.AIStatus != "completed") ||
			(check.HumanRequired && check.HumanStatus != "completed") {
			reasons = append(reasons, "check_incomplete:"+check.ID)
		}

		if check.HumanRequired && check.HumanEvidenceRequirement != "none" {
			found := false

			for _, evidence := range check.Evidence {
				if evidence.Actor == "human" && evidence.Kind == check.HumanEvidenceRequirement &&
					evidence.Status == "available" {
					found = true
				}
			}

			if !found {
				reasons = append(reasons, "evidence_missing:"+check.ID)
			}
		}
	}

	return len(reasons) == 0, reasons
}
