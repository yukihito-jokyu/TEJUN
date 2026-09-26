package acp

import (
	"context"
	"errors"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/yukihito-jokyu/TEJUN/internal/adapter/workspace"
	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

var _ application.ProjectSessionConnector = (*Manager)(nil)

type projectSession struct {
	process    *process
	connection *sdk.ClientSideConnection
	projectID  string
}

func (m *Manager) ConnectProject(
	ctx context.Context,
	job application.ClaimedProjectJob,
) (application.ProjectSessionConnected, error) {
	if job.SessionID == "" || job.WorkspacePath == "" || job.Connection.Command == "" {
		return application.ProjectSessionConnected{}, errors.New("project session input is incomplete")
	}

	if err := workspace.Validate(job.WorkspacePath); err != nil {
		return application.ProjectSessionConnected{}, err
	}

	initializeContext, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	stopLifecycleCancel := context.AfterFunc(m.lifecycle, cancel)
	defer stopLifecycleCancel()

	proc, stdout, err := startProcess(m.lifecycle, initializeContext, job.Connection.Command, job.Connection.Args,
		buildEnvironment(job.Connection.EnvironmentOverrides, nil))
	if err != nil {
		return application.ProjectSessionConnected{}, acpError(err, "start")
	}

	keep := false
	defer func() {
		if !keep {
			_ = proc.close()
		}
	}()

	generation := m.generation.Add(1)
	connection := sdk.NewClientSideConnection(&client{
		manager: m, attemptID: job.SessionID,
		generation: generation,
	}, proc.stdin, stdout)

	response, err := connection.Initialize(initializeContext, sdk.InitializeRequest{
		ProtocolVersion:    sdk.ProtocolVersionNumber,
		ClientCapabilities: sdk.ClientCapabilities{},
		ClientInfo:         &sdk.Implementation{Name: "tejun", Version: "1"},
	})
	if err != nil {
		return application.ProjectSessionConnected{}, acpError(err, "initialize")
	}

	if response.ProtocolVersion != sdk.ProtocolVersionNumber {
		return application.ProjectSessionConnected{}, &shared.Error{
			Code:      "acp_not_compatible",
			Message:   "AgentのACP protocol versionに互換性がありません",
			Retryable: true,
		}
	}

	capabilities := response.AgentCapabilities

	mode := projectRecoveryMode(job.Strategy, job.PreviousAgentSessionID, capabilities)
	if mode == "resume" && capabilities.SessionCapabilities.Resume == nil {
		return application.ProjectSessionConnected{}, &shared.Error{
			Code:      "acp_not_compatible",
			Message:   "Agentはsession resumeに対応していません",
			Retryable: true,
		}
	}

	if mode == "load" && !capabilities.LoadSession {
		return application.ProjectSessionConnected{}, &shared.Error{
			Code:      "acp_not_compatible",
			Message:   "Agentはsession loadに対応していません",
			Retryable: true,
		}
	}

	if mode != "new" && job.PreviousAgentSessionID == "" {
		return application.ProjectSessionConnected{}, errors.New("previous agent session is missing")
	}

	connected := application.ProjectSessionConnected{
		RecoveryMode:    mode,
		ResumeSupported: capabilities.SessionCapabilities.Resume != nil, LoadSupported: capabilities.LoadSession,
		ProcessGeneration: generation,
	}
	switch mode {
	case "resume":
		_, err = connection.ResumeSession(initializeContext, sdk.ResumeSessionRequest{
			Cwd:        job.WorkspacePath,
			McpServers: []sdk.McpServer{}, SessionId: sdk.SessionId(job.PreviousAgentSessionID),
		})
		connected.AgentSessionID = job.PreviousAgentSessionID
	case "load":
		_, err = connection.LoadSession(initializeContext, sdk.LoadSessionRequest{
			Cwd:        job.WorkspacePath,
			McpServers: []sdk.McpServer{}, SessionId: sdk.SessionId(job.PreviousAgentSessionID),
		})
		connected.AgentSessionID = job.PreviousAgentSessionID
	case "new":
		var newSession sdk.NewSessionResponse

		newSession, err = connection.NewSession(
			initializeContext,
			sdk.NewSessionRequest{Cwd: job.WorkspacePath, McpServers: []sdk.McpServer{}},
		)
		connected.AgentSessionID = string(newSession.SessionId)
	default:
		return application.ProjectSessionConnected{}, errors.New("unsupported recovery mode")
	}

	if err != nil {
		if initializeContext.Err() != nil {
			return application.ProjectSessionConnected{}, acpError(initializeContext.Err(), "session")
		}

		return application.ProjectSessionConnected{}, acpError(err, "session")
	}

	if connected.AgentSessionID == "" {
		return application.ProjectSessionConnected{}, &shared.Error{
			Code:      "acp_not_compatible",
			Message:   "Agentがsession IDを返しませんでした",
			Retryable: true,
		}
	}

	m.mu.Lock()
	oldSessions := make([]*projectSession, 0)

	for id, old := range m.projectSessions {
		if old.projectID == job.ProjectID {
			oldSessions = append(oldSessions, old)

			delete(m.projectSessions, id)
		}
	}

	m.projectSessions[job.SessionID] = &projectSession{process: proc, connection: connection, projectID: job.ProjectID}
	m.mu.Unlock()

	for _, old := range oldSessions {
		_ = old.process.close()
	}

	go func() {
		<-proc.done
		m.mu.Lock()
		if current := m.projectSessions[job.SessionID]; current != nil && current.process == proc {
			delete(m.projectSessions, job.SessionID)
		}
		m.mu.Unlock()
	}()

	keep = true

	return connected, nil
}

func projectRecoveryMode(strategy, previous string, capabilities sdk.AgentCapabilities) string {
	if strategy == "resume_existing" {
		return "resume"
	}

	if strategy == "auto" && previous != "" {
		if capabilities.SessionCapabilities.Resume != nil {
			return "resume"
		}

		if capabilities.LoadSession {
			return "load"
		}
	}

	return "new"
}
