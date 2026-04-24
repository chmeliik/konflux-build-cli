package cliwrappers_test

import (
	"errors"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
	"github.com/konflux-ci/konflux-build-cli/testutil"
)

func setupSubscriptionManagerCli() (*cliwrappers.SubscriptionManagerCli, *mockExecutor) {
	executor := &mockExecutor{}
	smCli := &cliwrappers.SubscriptionManagerCli{Executor: executor}
	return smCli, executor
}

func setGetUIDForTest(t *testing.T, uid int) {
	orig := *cliwrappers.ExportGetUID
	*cliwrappers.ExportGetUID = func() int { return uid }
	t.Cleanup(func() { *cliwrappers.ExportGetUID = orig })
}

func TestSubscriptionManagerCli_Register(t *testing.T) {
	g := NewWithT(t)
	ensureRetryerDisabled(t)

	t.Run("should register with org and activation key", func(t *testing.T) {
		setGetUIDForTest(t, 0)

		smCli, executor := setupSubscriptionManagerCli()
		var capturedArgs []string
		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			g.Expect(cmd.Name).To(Equal("subscription-manager"))
			capturedArgs = cmd.Args
			return "", "", 0, nil
		}

		params := &cliwrappers.SubscriptionManagerRegisterParams{
			Org:           "my-org",
			ActivationKey: "my-key",
		}

		err := smCli.Register(params)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(capturedArgs).To(Equal([]string{
			"register", "--org", "my-org", "--activationkey", "my-key",
		}))
	})

	t.Run("should include --force when Force is true", func(t *testing.T) {
		setGetUIDForTest(t, 0)

		smCli, executor := setupSubscriptionManagerCli()
		var capturedArgs []string
		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			capturedArgs = cmd.Args
			return "", "", 0, nil
		}

		params := &cliwrappers.SubscriptionManagerRegisterParams{
			Org:           "my-org",
			ActivationKey: "my-key",
			Force:         true,
		}

		err := smCli.Register(params)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(capturedArgs).To(Equal([]string{
			"register", "--force", "--org", "my-org", "--activationkey", "my-key",
		}))
	})

	t.Run("should return error when registration fails", func(t *testing.T) {
		setGetUIDForTest(t, 0)

		smCli, executor := setupSubscriptionManagerCli()
		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			return "", "", 1, errors.New("command failed")
		}

		params := &cliwrappers.SubscriptionManagerRegisterParams{
			Org:           "my-org",
			ActivationKey: "my-key",
		}

		err := smCli.Register(params)

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(Equal("command failed"))
	})

	t.Run("should return error when not root", func(t *testing.T) {
		setGetUIDForTest(t, 1000)

		smCli, _ := setupSubscriptionManagerCli()
		params := &cliwrappers.SubscriptionManagerRegisterParams{
			Org:           "my-org",
			ActivationKey: "my-key",
		}

		err := smCli.Register(params)

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("requires root"))
	})
}

func TestSubscriptionManagerCli_Unregister(t *testing.T) {
	g := NewWithT(t)

	t.Run("should unregister", func(t *testing.T) {
		smCli, executor := setupSubscriptionManagerCli()
		var capturedArgs []string
		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			g.Expect(cmd.Name).To(Equal("subscription-manager"))
			capturedArgs = cmd.Args
			return "", "", 0, nil
		}

		smCli.Unregister()

		g.Expect(capturedArgs).To(Equal([]string{"unregister"}))
	})

	t.Run("should log a warning on failure", func(t *testing.T) {
		smCli, executor := setupSubscriptionManagerCli()
		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			return "", "", 1, errors.New("unregister failed")
		}

		logOutput := testutil.CaptureLogOutput(smCli.Unregister)
		g.Expect(logOutput).To(ContainSubstring("subscription-manager unregister command failed"))
	})
}
