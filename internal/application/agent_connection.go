package application

import (
	"context"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/domain/agentconnection"
)

type StartupRepository interface {
	GetStartupState(context.Context) (StartupState, error)
	CompleteInitialSetup(context.Context, CompleteSetupRecord) (MutationResult[InitialSetupResult], error)
}

type CandidateScanner interface {
	List(context.Context, bool) (AgentCandidateResult, error)
}

type ProbeManager interface {
	Check(context.Context, CheckAuthenticationInput) (AgentProbeResult, error)
	Get(context.Context, string) (agentconnection.Probe, error)
	Fingerprint(agentconnection.ConnectionInput) string
}

type Startup struct {
	repository StartupRepository
	scanner    CandidateScanner
	probes     ProbeManager
	now        func() time.Time
	newID      func() string
}

func NewStartup(
	repository StartupRepository,
	scanner CandidateScanner,
	probes ProbeManager,
	now func() time.Time,
	newID func() string,
) *Startup {
	return &Startup{repository: repository, scanner: scanner, probes: probes, now: now, newID: newID}
}

type StartupState struct {
	InitialSetupRequired bool
	DefaultConnection    *agentconnection.Connection
	RecoveryNotice       string
	NextRoute            string
	ChangeSequence       int64
}

type AgentCandidate struct {
	CandidateKey           string
	DisplayName            string
	Command                string
	Args                   []string
	Transport              string
	Source                 string
	ResolvedExecutablePath string
	Warnings               []string
}

type AgentCandidateResult struct {
	Items     []AgentCandidate
	ScannedAt time.Time
	Warnings  []string
}

type CheckAuthenticationInput struct {
	Connection  agentconnection.ConnectionInput
	OperationID string
}

type AgentProbeResult struct {
	Probe        agentconnection.Probe
	AuthMethods  []AuthMethod
	Capabilities map[string]bool
}

type AuthMethod struct {
	Type             string
	ID               string
	Name             string
	Description      string
	Args             []string
	EnvironmentNames []string
}

type CompleteSetupInput struct {
	Connection  agentconnection.ConnectionInput
	ProbeID     string
	OperationID string
}

type CompleteSetupRecord struct {
	OperationID string
	RequestHash string
	Connection  agentconnection.Connection
	Event       OutboxEvent
	Receipt     MutationReceipt
}

type InitialSetupResult struct {
	Connection     agentconnection.Connection
	NextRoute      string
	ChangeSequence int64
}

func (s *Startup) GetState(ctx context.Context) (StartupState, error) {
	return s.repository.GetStartupState(ctx)
}

func (s *Startup) ListAgentCandidates(ctx context.Context, refresh bool) (AgentCandidateResult, error) {
	return s.scanner.List(ctx, refresh)
}

func (s *Startup) CheckAuthentication(ctx context.Context, input CheckAuthenticationInput) (AgentProbeResult, error) {
	return s.probes.Check(ctx, input)
}

func (s *Startup) CompleteInitialSetup(
	ctx context.Context,
	input CompleteSetupInput,
) (MutationResult[InitialSetupResult], error) {
	probe, err := s.probes.Get(ctx, input.ProbeID)
	if err != nil {
		return MutationResult[InitialSetupResult]{}, err
	}

	fingerprint := s.probes.Fingerprint(input.Connection)
	if err := probe.CanComplete(s.now(), fingerprint); err != nil {
		return MutationResult[InitialSetupResult]{}, err
	}

	id := s.newID()
	now := s.now()
	record := CompleteSetupRecord{
		OperationID: input.OperationID,
		RequestHash: fingerprint,
		Connection: agentconnection.Connection{
			ID: id, DisplayName: input.Connection.DisplayName, Command: input.Connection.Command,
			Args: input.Connection.Args, Transport: input.Connection.Transport,
			ResolvedExecutablePath: probe.ResolvedExecutablePath, LastVerifiedAt: now,
			ProtocolVersion: probe.ProtocolVersion, AuthState: probe.AuthState,
			SchemaArtifactVersion: "1", Revision: 1,
		},
		Receipt: MutationReceipt{OperationID: input.OperationID, CommittedAt: now},
		Event: OutboxEvent{
			ID: s.newID(), Name: "agent.connection.changed", AggregateType: "agent_connection",
			AggregateID: id, EmittedAt: now,
		},
	}

	return s.repository.CompleteInitialSetup(ctx, record)
}

type MutationReceipt struct {
	OperationID string
	CommittedAt time.Time
}

type MutationResult[T any] struct {
	Data    T
	Receipt MutationReceipt
}

type OutboxEvent struct {
	ID            string
	Name          string
	EmittedAt     time.Time
	AggregateType string
	AggregateID   string
	Correlation   string
}
