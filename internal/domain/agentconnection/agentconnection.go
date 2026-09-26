package agentconnection

import (
	"errors"
	"time"
)

type AuthState string

const (
	AuthUnknown       AuthState = "unknown"
	AuthNotRequired   AuthState = "not_required"
	AuthRequired      AuthState = "required"
	AuthAuthenticated AuthState = "authenticated"
	AuthFailed        AuthState = "failed"
	AuthUnsupported   AuthState = "unsupported"
	TransportStdio              = "stdio"
	CompatibilityOK             = "compatible"
)

type ConnectionInput struct {
	DisplayName          string
	Command              string
	Args                 []string
	Transport            string
	EnvironmentOverrides []EnvironmentVariable
}

type EnvironmentVariable struct {
	Name  string
	Value string
}

type Connection struct {
	ID                     string
	DisplayName            string
	Command                string
	Args                   []string
	Transport              string
	ResolvedExecutablePath string
	LastVerifiedAt         time.Time
	ProtocolVersion        string
	AuthState              AuthState
	SchemaArtifactVersion  string
	Revision               int64
}

type Probe struct {
	ID                     string
	ConfigFingerprint      string
	Compatibility          string
	ProtocolVersion        string
	ResolvedExecutablePath string
	AuthState              AuthState
	ProcessGeneration      int64
	ExpiresAt              time.Time
}

func (p Probe) CanComplete(now time.Time, fingerprint string) error {
	if !p.ExpiresAt.After(now) {
		return errors.New("probeの有効期限が切れています")
	}

	if p.ConfigFingerprint != fingerprint {
		return errors.New("probeと接続設定が一致しません")
	}

	if p.Compatibility != CompatibilityOK {
		return errors.New("ACP互換性を確認できません")
	}

	if p.AuthState != AuthAuthenticated && p.AuthState != AuthNotRequired {
		return errors.New("認証が完了していません")
	}

	return nil
}

type JobKind string

const (
	JobAuthenticate JobKind = "authenticate"
	JobLogout       JobKind = "logout"
)

type JobState string

const (
	JobPending   JobState = "pending"
	JobRunning   JobState = "running"
	JobSucceeded JobState = "succeeded"
	JobFailed    JobState = "failed"
)

func CanTransitionJob(from, to JobState) bool {
	switch from {
	case JobPending:
		return to == JobRunning
	case JobRunning:
		return to == JobSucceeded || to == JobFailed
	default:
		return false
	}
}

type ElicitationState string

const (
	ElicitationPending    ElicitationState = "pending"
	ElicitationResponding ElicitationState = "responding"
	ElicitationResponded  ElicitationState = "responded"
	ElicitationFailed     ElicitationState = "failed"
)

func CanTransitionElicitation(from, to ElicitationState) bool {
	switch from {
	case ElicitationPending:
		return to == ElicitationResponding
	case ElicitationResponding:
		return to == ElicitationResponded || to == ElicitationFailed
	default:
		return false
	}
}
