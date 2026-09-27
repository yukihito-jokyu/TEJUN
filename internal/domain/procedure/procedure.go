package procedure

import "strings"

type Status string

const (
	Generating Status = "generating"
	Draft      Status = "draft"
	Checking   Status = "checking"
	Completed  Status = "completed"
	Failed     Status = "failed"
)

type Document struct {
	Title         string   `json:"title"`
	Overview      string   `json:"overview"`
	Prerequisites []string `json:"prerequisites"`
	Steps         []Step   `json:"steps"`
}

type Step struct {
	ID           string        `json:"stepId,omitempty"`
	ClientKey    string        `json:"clientKey"`
	Title        string        `json:"title"`
	Description  string        `json:"description"`
	Command      string        `json:"command,omitempty"`
	Notes        []string      `json:"notes"`
	EvidenceRefs []EvidenceRef `json:"evidenceRefs"`
}

type EvidenceRef struct {
	EvidenceID  string `json:"evidenceId"`
	DisplayName string `json:"displayName"`
	Included    bool   `json:"included"`
}

type Integrity struct {
	Status string  `json:"status"`
	Issues []Issue `json:"issues"`
}

type Issue struct {
	Code       string `json:"code"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	StepID     string `json:"stepId,omitempty"`
	EvidenceID string `json:"evidenceId,omitempty"`
}

func Evaluate(document Document, sourceEvidenceIDs map[string]bool) Integrity {
	result := Integrity{Status: "valid", Issues: []Issue{}}
	add := func(code, stepID, evidenceID string) {
		result.Status = "blocked"
		result.Issues = append(result.Issues, Issue{
			Code: code, Severity: "blocking", Message: code, StepID: stepID, EvidenceID: evidenceID,
		})
	}

	if strings.TrimSpace(document.Title) == "" {
		add("title_required", "", "")
	}

	if len(document.Steps) == 0 {
		add("steps_required", "", "")
	}

	keys := make(map[string]bool, len(document.Steps))
	for _, step := range document.Steps {
		if strings.TrimSpace(step.ClientKey) == "" || keys[step.ClientKey] {
			add("invalid_client_key", step.ID, "")
		}

		keys[step.ClientKey] = true
		if strings.TrimSpace(step.Title) == "" {
			add("step_title_required", step.ID, "")
		}

		refs := make(map[string]bool, len(step.EvidenceRefs))
		for _, ref := range step.EvidenceRefs {
			if strings.TrimSpace(ref.EvidenceID) == "" || refs[ref.EvidenceID] || !sourceEvidenceIDs[ref.EvidenceID] {
				add("invalid_evidence_ref", step.ID, ref.EvidenceID)
			}

			refs[ref.EvidenceID] = true
		}
	}

	return result
}
