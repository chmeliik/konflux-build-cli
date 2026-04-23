package cliwrappers

import (
	"errors"

	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

var submanLog = l.Logger.WithField("logger", "SubscriptionManagerCli")

type SubscriptionManagerCliInterface interface {
	Register(params *SubscriptionManagerRegisterParams) error
	Unregister()
}

type SubscriptionManagerRegisterParams struct {
	Org           string
	ActivationKey string
	Force         bool
}

type SubscriptionManagerCli struct {
	Executor CliExecutorInterface
}

func NewSubscriptionManagerCli(executor CliExecutorInterface) (*SubscriptionManagerCli, error) {
	available, err := CheckCliToolAvailable("subscription-manager")
	if err != nil {
		return nil, err
	}
	if !available {
		return nil, errors.New("subscription-manager CLI is not available")
	}
	return &SubscriptionManagerCli{Executor: executor}, nil
}

// Register the system with Red Hat Subscription Manager.
func (sm *SubscriptionManagerCli) Register(params *SubscriptionManagerRegisterParams) error {
	args := []string{"register"}
	if params.Force {
		args = append(args, "--force")
	}
	args = append(args, "--org", params.Org, "--activationkey", params.ActivationKey)

	command := func() (string, string, int, error) {
		return sm.Executor.Execute(Cmd{Name: "subscription-manager", Args: args})
	}

	retryer := NewRetryer(command).StopIfOutputContains("unauthorized")
	_, stderr, _, err := retryer.Run()
	if err != nil {
		submanLog.Errorf("subscription-manager register failed: %s", err.Error())
		if stderr != "" {
			submanLog.Errorf("stderr:\n%s", stderr)
		}
		return err
	}
	return nil
}

// Unregister the system from Red Hat Subscription Manager (best-effort).
func (sm *SubscriptionManagerCli) Unregister() {
	_, stderr, _, err := sm.Executor.Execute(Cmd{Name: "subscription-manager", Args: []string{"unregister"}})
	if err != nil {
		submanLog.Warn("subscription-manager unregister command failed")
		if stderr != "" {
			submanLog.Warnf("stderr:\n%s", stderr)
		}
	}
}
